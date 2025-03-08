//go:build outbox_hw

package outbox_hw

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var db *sql.DB

const (
	authorTableName     = "author"
	bookTableName       = "book"
	authorBookTableName = "author_book"
	outboxTable         = "outbox"
)

func TestMain(m *testing.M) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	dbName := os.Getenv("POSTGRES_DB")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")

	source := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		url.QueryEscape(user),
		url.QueryEscape(password),
		host,
		port,
		dbName,
	)

	var err error
	db, err = sql.Open("postgres", source)

	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}

	code := m.Run()
	db.Close()
	os.Exit(code)
}

func cleanUp(t *testing.T) {
	t.Helper()

	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", authorTableName))
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", bookTableName))
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", authorBookTableName))
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", authorBookTableName))
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", outboxTable))
	require.NoError(t, err)
}

func TestOutboxDuringDownTime(t *testing.T) {
	ctx := context.Background()
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)
	http.DefaultClient.Timeout = time.Second * 1

	mux := http.NewServeMux()

	const authorPath = "/author"
	const authorCount = 10

	mux.HandleFunc("POST "+authorPath, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	})

	const bookPath = "/book"
	const bookCount = 100
	mux.HandleFunc("POST "+bookPath, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	})

	httpTestServer := httptest.NewServer(mux)
	outboxConf := defaultOutboxConfiguration(
		httpTestServer.URL+authorPath,
		httpTestServer.URL+bookPath,
	)

	outboxConf.WaitTimeMS = 100 * time.Millisecond
	outboxConf.BatchSize = 2
	outboxConf.InProgressTTLMS = 1000 * time.Millisecond
	outboxConf.Workers = 3

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, outboxConf)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	client := newGRPCClient(t, grpcPort)
	authorIDs := make([]string, 0, authorCount)

	for i := 0; i < authorCount; i++ {
		func() {
			author, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: "author" + strconv.Itoa(i),
			})
			require.NoError(t, err)
			require.NotEmpty(t, author.GetId())

			authorIDs = append(authorIDs, author.GetId())
		}()
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < bookCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			book, err := client.AddBook(ctx, &AddBookRequest{
				Name:      fmt.Sprint(rand.N[int](1e9)),
				AuthorIds: authorIDs[:rand.N(authorCount/3+1)],
			})

			require.NoError(t, err)
			require.NotEmpty(t, book.GetBook().GetId())
		}()
	}

	wg.Wait()
	time.Sleep(time.Second * 5)
}

func TestNoOpOutbox(t *testing.T) {
	ctx := context.Background()
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)
	http.DefaultClient.Timeout = time.Second * 1

	mux := http.NewServeMux()

	authorCounter := atomic.Int64{}

	const authorPath = "/author"
	const authorCount = 10
	mux.HandleFunc("POST "+authorPath, func(writer http.ResponseWriter, request *http.Request) {
		authorCounter.Add(1)
	})

	bookCounter := atomic.Int64{}
	const bookPath = "/book"
	const bookCount = 100
	mux.HandleFunc("POST "+bookPath, func(writer http.ResponseWriter, request *http.Request) {
		bookCounter.Add(1)
	})

	httpTestServer := httptest.NewServer(mux)
	outboxConf := defaultOutboxConfiguration(
		httpTestServer.URL+authorPath,
		httpTestServer.URL+bookPath,
	)

	outboxConf.WaitTimeMS = 10_000_000 * time.Millisecond
	outboxConf.BatchSize = 20
	outboxConf.InProgressTTLMS = 10_000_000 * time.Millisecond
	outboxConf.Workers = 3

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, outboxConf)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	client := newGRPCClient(t, grpcPort)
	authorIDs := make([]string, 0, authorCount)

	for i := 0; i < authorCount; i++ {
		func() {
			author, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: "author" + strconv.Itoa(i),
			})
			require.NoError(t, err)

			authorIDs = append(authorIDs, author.Id)
		}()
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < bookCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := client.AddBook(ctx, &AddBookRequest{
				Name:      fmt.Sprint(rand.N[int](1e9)),
				AuthorIds: authorIDs[:rand.N(authorCount/3+1)],
			})
			require.NoError(t, err)
		}()
	}

	time.Sleep(time.Second * 10)

	require.Equal(t, 0, int(authorCounter.Load()))
	require.Equal(t, 0, int(bookCounter.Load()))
}

func TestOutboxRetries(t *testing.T) {
	ctx := context.Background()
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)
	http.DefaultClient.Timeout = time.Second * 1

	mux := http.NewServeMux()

	authorCounter := atomic.Int64{}
	authorMx := new(sync.Mutex)
	authorMap := make(map[string]bool)
	const authorPath = "/author"
	const authorCount = 1000

	toggle := atomic.Bool{}

	mux.HandleFunc("POST "+authorPath, func(writer http.ResponseWriter, request *http.Request) {
		if !toggle.Load() && rand.N[int](2) == 1 {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		authorCounter.Add(1)

		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		authorID := string(data)

		authorMx.Lock()
		defer authorMx.Unlock()

		authorMap[authorID] = true
	})

	bookMx := new(sync.Mutex)
	bookMap := make(map[string]bool)
	bookCounter := atomic.Int64{}
	const bookPath = "/book"
	const bookCount = 100
	mux.HandleFunc("POST "+bookPath, func(writer http.ResponseWriter, request *http.Request) {
		if !toggle.Load() && rand.N[int](2) == 1 {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		bookCounter.Add(1)

		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		bookID := string(data)

		bookMx.Lock()
		defer bookMx.Unlock()

		bookMap[bookID] = true
	})

	httpTestServer := httptest.NewServer(mux)
	outboxConf := defaultOutboxConfiguration(
		httpTestServer.URL+authorPath,
		httpTestServer.URL+bookPath,
	)

	outboxConf.WaitTimeMS = 50 * time.Millisecond
	outboxConf.BatchSize = 20
	outboxConf.Workers = 5
	outboxConf.InProgressTTLMS = 1000 * time.Millisecond

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, outboxConf)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	client := newGRPCClient(t, grpcPort)
	authorIDs := make([]string, 0, authorCount)

	for i := 0; i < authorCount; i++ {
		func() {
			authorMx.Lock()
			defer authorMx.Unlock()

			registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: "author" + strconv.Itoa(i),
			})
			require.NoError(t, err)

			authorID := registerRes.GetId()
			authorIDs = append(authorIDs, authorID)

			v, ok := authorMap[authorID]

			// already deleted
			if ok && v {
				return
			}

			authorMap[authorID] = false
		}()
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < bookCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			bookMx.Lock()
			defer bookMx.Unlock()

			book, err := client.AddBook(ctx, &AddBookRequest{
				Name:      fmt.Sprint(rand.N[int](1e9)),
				AuthorIds: authorIDs[:rand.N(authorCount/3+1)],
			})
			require.NoError(t, err)

			bookID := book.GetBook().GetId()
			v, ok := bookMap[bookID]

			// already deleted
			if ok && v {
				return
			}

			bookMap[bookID] = false
		}()
	}

	wg.Wait()
	toggle.Store(true)
	time.Sleep(time.Second * 20)

	authorMx.Lock()
	defer authorMx.Unlock()

	for _, v := range authorMap {
		require.True(t, v)
	}

	bookMx.Lock()
	defer bookMx.Unlock()

	for _, v := range bookMap {
		require.True(t, v)
	}

	require.GreaterOrEqual(t, int(authorCounter.Load()), authorCount)
	require.GreaterOrEqual(t, int(bookCounter.Load()), bookCount)
}

func TestOutboxConsistency(t *testing.T) {
	ctx := context.Background()
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)
	http.DefaultClient.Timeout = time.Second * 1

	mux := http.NewServeMux()

	authorMx := new(sync.Mutex)
	authorMap := make(map[string]bool)
	const authorPath = "/author"
	const authorCount = 100
	mux.HandleFunc("POST "+authorPath, func(writer http.ResponseWriter, request *http.Request) {
		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		authorID := string(data)

		authorMx.Lock()
		defer authorMx.Unlock()

		authorMap[authorID] = true
	})

	bookMx := new(sync.RWMutex)
	bookMap := make(map[string]bool)
	bookCounter := atomic.Int64{}
	const bookPath = "/book"
	mux.HandleFunc("POST "+bookPath, func(writer http.ResponseWriter, request *http.Request) {
		bookCounter.Add(1)

		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		bookID := string(data)

		bookMx.Lock()
		defer bookMx.Unlock()

		bookMap[bookID] = true
	})

	httpTestServer := httptest.NewServer(mux)
	outboxConf := defaultOutboxConfiguration(
		httpTestServer.URL+authorPath,
		httpTestServer.URL+bookPath,
	)
	outboxConf.BatchSize = 25
	outboxConf.Workers = 7
	outboxConf.WaitTimeMS = 100 * time.Millisecond
	outboxConf.InProgressTTLMS = 1000 * time.Millisecond

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, outboxConf)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	client := newGRPCClient(t, grpcPort)
	authorIDs := make([]string, 0, authorCount)

	for i := 0; i < authorCount; i++ {
		func() {
			authorMx.Lock()
			defer authorMx.Unlock()

			registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: "author" + strconv.Itoa(i),
			})
			require.NoError(t, err)

			authorID := registerRes.GetId()
			authorIDs = append(authorIDs, authorID)
		}()
	}

	wg := new(sync.WaitGroup)
	errCounter := atomic.Int64{}
	const iterations = 100

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			if t.Failed() {
				break
			}

			if errCounter.Load() >= iterations {
				break
			}

			select {
			case <-time.Tick(time.Millisecond * 40):
				stopLibrary(t, cmd)
				cmd = setupLibrary(t, executable, grpcPort, grpcGatewayPort, outboxConf)
			}
		}
	}()

	for range runtime.GOMAXPROCS(-1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if errCounter.Load() >= iterations {
					return
				}

				name := fmt.Sprint(rand.N[int](1e9))
				addedBook, err := client.AddBook(ctx, &AddBookRequest{
					Name:      name,
					AuthorIds: authorIDs[:rand.N(authorCount/3+1)],
				})

				time.Sleep(time.Millisecond * 50)
				if err != nil {
					errCounter.Add(1)
					continue
				} else {
					require.Equal(t, name, addedBook.GetBook().GetName())
					require.NotEmpty(t, name, addedBook.GetBook().GetId())
				}
			}
		}()
	}

	wg.Wait()
	time.Sleep(time.Second * 20)

	checkWg := new(sync.WaitGroup)
	for _, aID := range authorIDs {
		checkWg.Add(1)
		go func() {
			defer checkWg.Done()
			bookMx.RLock()
			defer bookMx.RUnlock()

			if t.Failed() {
				return
			}

			books := getAllAuthorBooks(t, aID, client)

			for _, b := range books {
				v, ok := bookMap[b.GetId()]

				require.NotEmpty(t, b.GetId())
				require.True(t, ok, b.GetId())
				require.True(t, v, b.GetId())
			}
		}()
	}

	checkWg.Wait()
}

func TestOutbox(t *testing.T) {
	ctx := context.Background()
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)
	http.DefaultClient.Timeout = time.Second * 1

	mux := http.NewServeMux()

	authorCounter := atomic.Int64{}
	authorMx := new(sync.Mutex)
	authorMap := make(map[string]bool)
	const authorPath = "/author"
	const authorCount = 10
	mux.HandleFunc("POST "+authorPath, func(writer http.ResponseWriter, request *http.Request) {
		authorCounter.Add(1)

		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		authorID := string(data)

		authorMx.Lock()
		defer authorMx.Unlock()

		authorMap[authorID] = true
	})

	bookMx := new(sync.Mutex)
	bookMap := make(map[string]bool)
	bookCounter := atomic.Int64{}
	const bookPath = "/book"
	const bookCount = 100
	mux.HandleFunc("POST "+bookPath, func(writer http.ResponseWriter, request *http.Request) {
		bookCounter.Add(1)

		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		bookID := string(data)

		bookMx.Lock()
		defer bookMx.Unlock()

		bookMap[bookID] = true
	})

	httpTestServer := httptest.NewServer(mux)
	outboxConf := defaultOutboxConfiguration(
		httpTestServer.URL+authorPath,
		httpTestServer.URL+bookPath,
	)

	outboxConf.WaitTimeMS = 100 * time.Millisecond
	outboxConf.BatchSize = 20
	outboxConf.InProgressTTLMS = time.Millisecond * 1_000_000
	outboxConf.Workers = 3

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, outboxConf)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	client := newGRPCClient(t, grpcPort)
	authorIDs := make([]string, 0, authorCount)

	for i := 0; i < authorCount; i++ {
		func() {
			authorMx.Lock()
			defer authorMx.Unlock()

			registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: "author" + strconv.Itoa(i),
			})
			require.NoError(t, err)

			authorID := registerRes.GetId()
			authorIDs = append(authorIDs, authorID)

			v, ok := authorMap[authorID]

			// already deleted
			if ok && v {
				return
			}

			authorMap[authorID] = false
			time.Sleep(time.Millisecond * 100)
		}()
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < bookCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			bookMx.Lock()
			defer bookMx.Unlock()

			book, err := client.AddBook(ctx, &AddBookRequest{
				Name:      fmt.Sprint(rand.N[int](1e9)),
				AuthorIds: authorIDs[:rand.N(authorCount/3+1)],
			})
			require.NoError(t, err)

			bookID := book.GetBook().GetId()
			v, ok := bookMap[bookID]

			// already deleted
			if ok && v {
				return
			}

			bookMap[bookID] = false
			time.Sleep(time.Millisecond * 100)
		}()
	}

	time.Sleep(time.Second * 15)

	authorMx.Lock()
	defer authorMx.Unlock()

	for _, v := range authorMap {
		require.True(t, v)
	}

	bookMx.Lock()
	defer bookMx.Unlock()

	for _, v := range bookMap {
		require.True(t, v)
	}

	require.Equal(t, authorCount, int(authorCounter.Load()))
	require.Equal(t, bookCount, int(bookCounter.Load()))
}

func defaultOutboxConfiguration(authorURL, bookURL string) *outBoxConfiguration {
	return &outBoxConfiguration{
		Workers:         2,
		BatchSize:       2,
		WaitTimeMS:      time.Millisecond * 3000,
		InProgressTTLMS: time.Millisecond * 10000,
		AuthorSendURL:   authorURL,
		BookSendURL:     bookURL,
	}
}

func TestLibraryConsistency(t *testing.T) {
	ctx := context.Background()
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)
	client := newGRPCClient(t, grpcPort)

	http.DefaultClient.Timeout = time.Second * 1

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, nil)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	const iterations = 100

	authorIDs := make([]string, 0, 100)
	for i := 0; i < 20; i++ {
		registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: "author" + strconv.Itoa(i),
		})
		require.NoError(t, err)

		authorID := registerRes.GetId()
		authorIDs = append(authorIDs, authorID)
	}

	slices.Sort(authorIDs)

	var (
		firstBookName      = "book-1"
		firstBookAuthorIDs = authorIDs[:15]
	)

	var (
		secondBookName      = "book-2"
		secondBookAuthorIDs = authorIDs[5:20]
	)

	// initially insert the first state
	book, err := client.AddBook(ctx, &AddBookRequest{
		Name:      firstBookName,
		AuthorIds: firstBookAuthorIDs,
	})
	require.NoError(t, err)

	bookID := book.GetBook().GetId()

	wg := new(sync.WaitGroup)
	errCounter := atomic.Int64{}
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				if t.Failed() {
					break
				}

				if errCounter.Load() >= iterations {
					break
				}

				finalBook, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
					Id: bookID,
				})

				if err == nil {
					finalAuthorIDs := finalBook.GetBook().GetAuthorId()

					slices.Sort(finalAuthorIDs)

					if finalBook.GetBook().Name == firstBookName {
						require.Equal(t, firstBookAuthorIDs, finalAuthorIDs, "First book")
					}

					if finalBook.GetBook().Name == secondBookName {
						require.Equal(t, secondBookAuthorIDs, finalAuthorIDs, "Second book")
					}
				}

				var (
					newbookName      string
					newbookAuthorIDs []string
				)

				if rand.N[int](1_000_1234)%2 == 0 {
					newbookName = firstBookName
					newbookAuthorIDs = firstBookAuthorIDs
				} else {
					newbookName = secondBookName
					newbookAuthorIDs = secondBookAuthorIDs
				}

				_, err = client.UpdateBook(ctx, &UpdateBookRequest{
					Id:        bookID,
					Name:      newbookName,
					AuthorIds: newbookAuthorIDs,
				})

				if err != nil {
					errCounter.Add(1)
					time.Sleep(time.Millisecond * 300)
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			if t.Failed() {
				break
			}

			if errCounter.Load() >= iterations {
				break
			}

			select {
			case <-time.Tick(time.Millisecond * 500):
				stopLibrary(t, cmd)
				cmd = setupLibrary(t, executable, grpcPort, grpcGatewayPort, nil)
			}
		}
	}()

	wg.Wait()
	time.Sleep(time.Second * 3)
}

func TestLibraryWithoutInMemoryInvariant(t *testing.T) {
	executable := getLibraryExecutable(t)
	grpcPort := findFreePort(t)
	grpcGatewayPort := findFreePort(t)

	http.DefaultClient.Timeout = time.Second * 1

	cmd := setupLibrary(t, executable, grpcPort, grpcGatewayPort, nil)
	t.Cleanup(func() {
		stopLibrary(t, cmd)
		cleanUp(t)
	})

	t.Run("author grpc", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		const authorName = "Test testovich"

		registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: authorName,
		})

		require.NoError(t, err)
		authorID := registerRes.GetId()

		author, err := client.GetAuthorInfo(ctx, &GetAuthorInfoRequest{
			Id: authorID,
		})

		require.NoError(t, err)
		require.Equal(t, authorName, author.GetName())
		require.Equal(t, authorID, author.GetId())

		_, err = client.ChangeAuthorInfo(ctx, &ChangeAuthorInfoRequest{
			Id:   authorID,
			Name: authorName + "123",
		})
		require.NoError(t, err)

		stopLibrary(t, cmd)
		cmd = setupLibrary(t, executable, grpcPort, grpcGatewayPort, nil)

		newAuthor, err := client.GetAuthorInfo(ctx, &GetAuthorInfoRequest{
			Id: authorID,
		})
		require.NoError(t, err)

		require.Equal(t, authorName+"123", newAuthor.GetName())
		require.Equal(t, authorID, newAuthor.GetId())
	})

	t.Run("book grpc", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		const (
			authorName = "Test testovich"
			bookName   = "go"
		)

		registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: authorName,
		})

		stopLibrary(t, cmd)
		cmd = setupLibrary(t, executable, grpcPort, grpcGatewayPort, nil)

		require.NoError(t, err)
		authorID := registerRes.GetId()

		createdTime := time.Now()
		time.Sleep(time.Second)
		response, err := client.AddBook(ctx, &AddBookRequest{
			Name:      bookName,
			AuthorIds: []string{authorID},
		})
		require.NoError(t, err)

		book := response.GetBook()

		require.Equal(t, bookName, book.GetName())
		require.Equal(t, 1, len(book.GetAuthorId()))
		require.Equal(t, authorID, book.GetAuthorId()[0])
		require.LessOrEqual(t, createdTime, book.GetCreatedAt().AsTime())

		_, err = client.UpdateBook(ctx, &UpdateBookRequest{
			Id:        book.GetId(),
			Name:      bookName + "-2024",
			AuthorIds: []string{authorID},
		})
		require.NoError(t, err)

		newBook, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
			Id: book.GetId(),
		})

		require.NoError(t, err)
		require.Equal(t, bookName+"-2024", newBook.GetBook().GetName())
		require.Equal(t, 1, len(newBook.GetBook().GetAuthorId()))
		require.Equal(t, authorID, newBook.GetBook().GetAuthorId()[0])
		require.LessOrEqual(t, book.CreatedAt.AsTime(), newBook.GetBook().GetUpdatedAt().AsTime())

		books := getAllAuthorBooks(t, authorID, client)

		require.NoError(t, err)
		require.Equal(t, 1, len(books))

		require.Equal(t, newBook.GetBook().GetName(), books[0].GetName())
		require.Equal(t, newBook.GetBook().GetAuthorId(), books[0].GetAuthorId())
	})

	t.Run("concurrent access", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		var (
			authorName = "Test testovich" + strconv.Itoa(rand.N[int](10e9))
			totalBooks = 1234
			workers    = 50
		)

		registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: authorName,
		})

		require.NoError(t, err)
		authorID := registerRes.GetId()

		books := make([]string, 0, totalBooks)
		for i := range totalBooks {
			books = append(books, strconv.Itoa(i))
		}

		perWorker := totalBooks / workers
		start := 0

		wg := new(sync.WaitGroup)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(s int) {
				defer wg.Done()

				right := s + perWorker
				if i == workers-1 {
					right = len(books)
				}

				for b := s; b < right; b++ {
					_, err := client.AddBook(ctx, &AddBookRequest{
						Name:      books[b],
						AuthorIds: []string{authorID},
					})
					require.NoError(t, err)
				}
			}(start)

			start += perWorker
		}

		wg.Wait()

		authorBooks := lo.Map(getAllAuthorBooks(t, authorID, client), func(item *Book, index int) string {
			return item.GetName()
		})

		slices.Sort(authorBooks)
		slices.Sort(books)

		require.Equal(t, books, authorBooks)
	})

	t.Run("author not found", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.GetAuthorInfo(ctx, &GetAuthorInfoRequest{
			Id: uuid.New().String(),
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.NotFound, s.Code())
	})

	t.Run("author invalid argument", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.GetAuthorInfo(ctx, &GetAuthorInfoRequest{
			Id: "123",
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, s.Code())
	})

	t.Run("book not found", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
			Id: uuid.New().String(),
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.NotFound, s.Code())
	})

	// https://www.pravmir.ru/russkie-narodnye-skazki/
	t.Run("book without author", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		response, err := client.AddBook(ctx, &AddBookRequest{
			Name:      "123",
			AuthorIds: nil,
		})
		require.NoError(t, err)

		book, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
			Id: response.GetBook().Id,
		})
		require.NoError(t, err)

		require.EqualExportedValues(t, response.GetBook(), book.GetBook())
	})

	t.Run("author_name validation error length", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: "",
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, s.Code())

		_, err = client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: strings.Repeat("N", 1024),
		})

		s, ok = status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, s.Code())
	})

	t.Run("author_name validation error pattern", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: "!!!",
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, s.Code())
	})

	t.Run("add book with not existing authors", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.AddBook(ctx, &AddBookRequest{
			Name:      "Test book",
			AuthorIds: []string{uuid.New().String()},
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.NotFound, s.Code())
	})

	t.Run("update book with not existing authors", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		const (
			authorName = "TestAuthor"
			bookName   = "BookForUpdate"
		)
		registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
			Name: authorName,
		})
		require.NoError(t, err)
		authorID := registerRes.GetId()
		addRes, err := client.AddBook(ctx, &AddBookRequest{
			Name:      bookName,
			AuthorIds: []string{authorID},
		})

		require.Equal(t, []string{registerRes.GetId()}, addRes.GetBook().GetAuthorId())

		bookId := addRes.GetBook().GetId()

		_, err = client.UpdateBook(ctx, &UpdateBookRequest{
			Id:        bookId,
			Name:      "Test book",
			AuthorIds: []string{uuid.NewString()},
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.NotFound, s.Code())
	})

	t.Run("book invalid argument", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		_, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
			Id: "123",
		})

		s, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, s.Code())
	})

	t.Run("grpc gateway", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		type RegisterAuthorResponse struct {
			ID string `json:"id"`
		}

		type GetAuthorResponse struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		registerUrl := fmt.Sprintf("http://127.0.0.1:%s/v1/library/author", grpcGatewayPort)

		request, err := http.NewRequest("POST", registerUrl, strings.NewReader(`{"name": "Name"}`))
		require.NoError(t, err)

		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)

		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		var registerAuthorResponse RegisterAuthorResponse

		err = json.Unmarshal(data, &registerAuthorResponse)
		require.NoError(t, err)

		require.NotEmpty(t, registerAuthorResponse)

		getUrl := fmt.Sprintf("http://127.0.0.1:%s/v1/library/author/%s",
			grpcGatewayPort, registerAuthorResponse.ID)

		getRequest, err := http.NewRequest("GET", getUrl, nil)
		require.NoError(t, err)

		getResponse, err := http.DefaultClient.Do(getRequest)
		require.NoError(t, err)

		getData, err := io.ReadAll(getResponse.Body)
		require.NoError(t, err)

		var author GetAuthorResponse
		err = json.Unmarshal(getData, &author)
		require.NoError(t, err)

		require.Equal(t, author.ID, registerAuthorResponse.ID)
		require.Equal(t, author.Name, "Name")
	})

	t.Run("book many authors grpc", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		var (
			authorBasicName = "Donald Knuth"
			authorsCount    = 10
			bookName        = "The Art of Computer Programming"
		)

		authorIds := make([]string, authorsCount)
		for i := range authorsCount {
			author, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: authorBasicName + strconv.Itoa(rand.N[int](10e9)),
			})
			require.NoError(t, err)
			authorIds[i] = author.Id
		}

		bookAdded, err := client.AddBook(ctx, &AddBookRequest{
			Name:      bookName,
			AuthorIds: authorIds,
		})
		require.NoError(t, err)
		require.ElementsMatch(t, bookAdded.Book.AuthorId, authorIds)

		bookReceived, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
			Id: bookAdded.Book.Id,
		})

		require.NoError(t, err)

		slices.Sort(bookAdded.Book.AuthorId)
		slices.Sort(bookReceived.Book.AuthorId)

		require.EqualExportedValues(t, bookAdded.Book, bookReceived.Book)
	})

	t.Run("update book changes GetAuthorBooks response", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		const bookName = "Book name"
		authorNames := []string{"Author1", "Author2"}
		authorIds := make([]string, 0, 2)

		for _, authorName := range authorNames {
			registerRes, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: authorName,
			})
			require.NoError(t, err)
			authorIds = append(authorIds, registerRes.GetId())
		}

		addRes, err := client.AddBook(ctx, &AddBookRequest{
			Name:      bookName,
			AuthorIds: authorIds,
		})
		require.NoError(t, err)
		book := addRes.GetBook()

		checkAuthorBooks := func(authorId string, book *Book) {
			books := getAllAuthorBooks(t, authorId, client)

			if book != nil {
				require.Equal(t, 1, len(books))
				require.Equal(t, book.GetName(), books[0].GetName())
				require.Contains(t, books[0].GetAuthorId(), authorId)
			} else {
				require.Equal(t, 0, len(books))
			}
		}

		checkAuthorBooks(authorIds[0], book)
		checkAuthorBooks(authorIds[1], book)

		_, err = client.UpdateBook(ctx, &UpdateBookRequest{
			Id:        book.GetId(),
			Name:      book.GetName(),
			AuthorIds: []string{authorIds[0]},
		})
		require.NoError(t, err)

		checkAuthorBooks(authorIds[0], book)
		checkAuthorBooks(authorIds[1], nil)
	})

	t.Run("update book concurrent calls", func(t *testing.T) {
		t.Cleanup(func() {
			cleanUp(t)
		})

		ctx := context.Background()
		client := newGRPCClient(t, grpcPort)

		const (
			bookName        = "Book name"
			authorBasicName = "Author"
			authorsCount    = 10
			iterations      = 100
			workers         = 50
		)

		authorIds := make([]string, authorsCount)
		for i := range authorsCount {
			author, err := client.RegisterAuthor(ctx, &RegisterAuthorRequest{
				Name: authorBasicName + strconv.Itoa(rand.N[int](1e9)),
			})
			require.NoError(t, err)
			authorIds[i] = author.Id
		}

		addRes, err := client.AddBook(ctx, &AddBookRequest{
			Name:      bookName,
			AuthorIds: authorIds,
		})
		require.NoError(t, err)
		book := addRes.GetBook()
		require.ElementsMatch(t, authorIds, book.GetAuthorId())

		// Now randomly update authors list in hope to break synchronization

		wg := new(sync.WaitGroup)
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()

				newAuthorIds := make([]string, 0)
				for _, authorId := range authorIds {
					if rand.N[int](1e9)%2 == 0 {
						newAuthorIds = append(newAuthorIds, authorId)
					}
				}

				for range iterations {
					_, err := client.UpdateBook(ctx, &UpdateBookRequest{
						Id:        book.GetId(),
						Name:      book.GetName(),
						AuthorIds: newAuthorIds,
					})
					require.NoError(t, err)
				}
			}()
		}

		wg.Wait()

		bookUpdated, err := client.GetBookInfo(ctx, &GetBookInfoRequest{
			Id: book.GetId(),
		})
		require.NoError(t, err)
		bookUpdatedAuthors := bookUpdated.GetBook().GetAuthorId()

		// check authorship consistency

		for _, authorId := range bookUpdatedAuthors {
			books := getAllAuthorBooks(t, authorId, client)

			require.Equal(t, 1, len(books))
			require.Equal(t, book.GetId(), books[0].GetId())
			require.ElementsMatch(t, bookUpdatedAuthors, books[0].GetAuthorId())
		}

		for _, authorId := range authorIds {
			if !slices.Contains(bookUpdatedAuthors, authorId) {
				books := getAllAuthorBooks(t, authorId, client)
				require.Equal(t, 0, len(books))
			}
		}
	})
}

func getLibraryExecutable(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	binaryPath, err := resolveFilePath(filepath.Dir(filepath.Dir(wd)), "library")
	require.NoError(t, err, "you need to compile your library service, run make build")

	return binaryPath
}

var requiredEnv = []string{"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_MAX_CONN"}

//var requiredEnv = make([]string, 0)

type outBoxConfiguration struct {
	Enabled         bool          `env:"OUTBOX_ENABLED"`
	Workers         int           `env:"OUTBOX_WORKERS"`
	BatchSize       int           `env:"OUTBOX_BATCH_SIZE"`
	WaitTimeMS      time.Duration `env:"OUTBOX_WAIT_TIME_MS"`
	InProgressTTLMS time.Duration `env:"OUTBOX_IN_PROGRESS_TTL_MS"`
	AuthorSendURL   string        `env:"OUTBOX_AUTHOR_SEND_URL"`
	BookSendURL     string        `env:"OUTBOX_BOOK_SEND_URL"`
}

func setupLibrary(
	t *testing.T,
	executable string,
	grpcPort string,
	grpcGatewayPort string,
	outboxCfg *outBoxConfiguration,
) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(executable)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	for _, p := range requiredEnv {
		cur := os.Getenv(p)
		require.NotEmpty(t, cur, "you need to pass env variable to tests: "+p)

		cmd.Env = append(cmd.Env, p+"="+cur)
	}

	cmd.Env = append(cmd.Env, "GRPC_PORT="+grpcPort)
	cmd.Env = append(cmd.Env, "GRPC_GATEWAY_PORT="+grpcGatewayPort)

	if outboxCfg == nil {
		cmd.Env = append(cmd.Env, "OUTBOX_ENABLED=false")
	} else {
		cmd.Env = append(cmd.Env, "OUTBOX_ENABLED=true")

		cmd.Env = append(cmd.Env, "OUTBOX_WORKERS="+fmt.Sprint(outboxCfg.Workers))
		cmd.Env = append(cmd.Env, "OUTBOX_BATCH_SIZE="+fmt.Sprint(outboxCfg.BatchSize))
		cmd.Env = append(cmd.Env, "OUTBOX_WAIT_TIME_MS="+fmt.Sprint(outboxCfg.WaitTimeMS.Milliseconds()))
		cmd.Env = append(cmd.Env, "OUTBOX_IN_PROGRESS_TTL_MS="+fmt.Sprint(outboxCfg.InProgressTTLMS.Milliseconds()))
		cmd.Env = append(cmd.Env, "OUTBOX_AUTHOR_SEND_URL="+outboxCfg.AuthorSendURL)
		cmd.Env = append(cmd.Env, "OUTBOX_BOOK_SEND_URL="+outboxCfg.BookSendURL)
	}

	require.NoError(t, cmd.Start())
	grpcClient := newGRPCClient(t, grpcPort)

	// grpc health check
	for i := range 50 {
		// use idempotent request and validation for healthcheck
		_, err := grpcClient.GetBookInfo(context.Background(), &GetBookInfoRequest{
			Id: "123", // not invalid
		})

		_, ok := status.FromError(err)

		if ok {
			break
		}

		if i == 19 {
			log.Println("grpc health check error")
			t.Fail()
		}

		time.Sleep(time.Millisecond * 100)
	}

	// gateway health check
	getUrl := fmt.Sprintf("http://127.0.0.1:%s/v1/library/author/%s", grpcGatewayPort, uuid.New())
	for i := range 50 {
		response, _ := http.Get(getUrl)

		if response != nil {
			break
		}

		if i == 29 {
			log.Println("gateway health check error")
			t.Fail()
		}

		time.Sleep(time.Millisecond * 100)
	}

	return cmd
}

func stopLibrary(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	for i := 0; i < 5; i++ {
		require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	}

	require.NoError(t, cmd.Wait())
	require.Equal(t, 0, cmd.ProcessState.ExitCode())
}

func findFreePort(t *testing.T) string {
	t.Helper()

	for {
		port := rand.N(16383) + 49152
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)

		if err == nil {
			require.NoError(t, ln.Close())
			return strconv.Itoa(port)
		}
	}
}

func newGRPCClient(t *testing.T, grpcPort string) LibraryClient {
	t.Helper()

	addr := "127.0.0.1:" + grpcPort
	c, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return NewLibraryClient(c)
}

func getAllAuthorBooks(t *testing.T, authorID string, client LibraryClient) []*Book {
	t.Helper()
	ctx := context.Background()

	result := make([]*Book, 0)
	stream, err := client.GetAuthorBooks(ctx, &GetAuthorBooksRequest{
		AuthorId: authorID,
	})
	require.NoError(t, err)

	for {
		resp, err := stream.Recv()

		if err == io.EOF {
			return result
		}

		require.NoError(t, err)

		result = append(result, resp)
	}
}

func resolveFilePath(root string, filename string) (string, error) {
	cleanedRoot := filepath.Clean(root)
	nameWithoutExt := strings.TrimRight(root, filepath.Ext(filename))

	var result string

	err := filepath.WalkDir(cleanedRoot, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		name := d.Name()

		if name == filename || name == nameWithoutExt {
			result = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("walk fail tree fail, error: %w", err)
	}

	if result == "" {
		return "", fmt.Errorf("file %s not found in root %s", filename, root)
	}

	return result, nil
}
