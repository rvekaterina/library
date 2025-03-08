package library

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rvekaterina/library/internal/entity"
	"github.com/rvekaterina/library/internal/usecase/repository"
	mocks "github.com/rvekaterina/library/internal/usecase/repository/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupBookUsecase(t *testing.T) (*libraryImpl, *mocks.MockBookRepository, *mocks.MockOutboxRepository, *mocks.MockTransactor) {
	t.Helper()
	controller := gomock.NewController(t)
	repo := mocks.NewMockBookRepository(controller)
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	logger, _ := zap.NewProduction()
	impl := New(logger, nil, repo, outboxRepo, transactor)
	return impl, repo, outboxRepo, transactor
}

func TestAddBook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bookName    string
		authorIDs   []string
		setupMocks  func(*mocks.MockBookRepository, *mocks.MockOutboxRepository, *mocks.MockTransactor)
		expected    entity.Book
		expectedErr error
	}{
		{
			name:      "Success",
			bookName:  "name",
			authorIDs: []string{"1", "2"},
			setupMocks: func(bookRepository *mocks.MockBookRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				book := entity.Book{
					ID:        "123",
					Name:      "name",
					AuthorIDs: []string{"1", "2"},
				}

				transactor.EXPECT().
					WithTx(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					}).
					Times(1)

				bookRepository.EXPECT().
					CreateBook(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, b entity.Book) (entity.Book, error) {
						require.Equal(t, "name", b.Name)
						require.Equal(t, []string{"1", "2"}, b.AuthorIDs)
						return book, nil
					}).
					Times(1)

				serialized, _ := json.Marshal(book)
				outboxRepository.EXPECT().
					SendMessage(
						gomock.Any(),
						repository.OutboxKindBook.String()+" "+book.ID,
						repository.OutboxKindBook,
						serialized,
					).
					Return(nil).
					Times(1)
			},
			expected: entity.Book{
				ID:        "123",
				Name:      "name",
				AuthorIDs: []string{"1", "2"},
			},
		},
		{
			name:      "Book repository error",
			bookName:  "name",
			authorIDs: []string{"1"},
			setupMocks: func(bookRepository *mocks.MockBookRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				transactor.EXPECT().
					WithTx(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					}).
					Times(1)

				bookRepository.EXPECT().
					CreateBook(gomock.Any(), entity.Book{
						Name:      "name",
						AuthorIDs: []string{"1"}}).
					Return(entity.Book{}, entity.ErrBookAlreadyExists).
					Times(1)

				outboxRepository.EXPECT().SendMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedErr: entity.ErrBookAlreadyExists,
		},
		{
			name:      "Outbox error",
			bookName:  "name",
			authorIDs: []string{"1"},
			setupMocks: func(bookRepository *mocks.MockBookRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				book := entity.Book{
					Name:      "name",
					AuthorIDs: []string{"1"},
					ID:        "123",
				}

				transactor.EXPECT().
					WithTx(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					}).
					Times(1)

				bookRepository.EXPECT().
					CreateBook(gomock.Any(), entity.Book{
						Name:      book.Name,
						AuthorIDs: book.AuthorIDs}).
					Return(book, nil).
					Times(1)

				serialized, _ := json.Marshal(book)
				outboxRepository.EXPECT().
					SendMessage(gomock.Any(), repository.OutboxKindBook.String()+" "+book.ID, repository.OutboxKindBook, serialized).
					Return(errOutbox).
					Times(1)
			},
			expectedErr: errOutbox,
		},
		{
			name:      "Transaction error",
			bookName:  "name",
			authorIDs: []string{"1"},
			setupMocks: func(bookRepository *mocks.MockBookRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				transactor.EXPECT().
					WithTx(gomock.Any(), gomock.Any()).
					Return(errTransactor).
					Times(1)

				bookRepository.EXPECT().CreateBook(gomock.Any(), gomock.Any()).Times(0)
				outboxRepository.EXPECT().SendMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedErr: errTransactor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, bookRepo, outboxRepo, transactor := setupBookUsecase(t)

			tc.setupMocks(bookRepo, outboxRepo, transactor)

			res, err := impl.RegisterBook(context.Background(), tc.bookName, tc.authorIDs)

			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.expected, res)
			}
		})
	}
}

func TestGetBook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		book entity.Book
		err  error
	}{
		{
			name: "Success",
			book: entity.Book{
				ID: "1",
			},
			err: nil,
		},
		{
			name: "Error book not found",
			book: entity.Book{},
			err:  entity.ErrBookNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, repo, _, _ := setupBookUsecase(t)

			repo.EXPECT().
				GetBook(context.Background(), tc.book.ID).
				Return(tc.book, tc.err).
				Times(1)
			res, err := impl.GetBook(context.Background(), tc.book.ID)
			require.Equal(t, tc.err, err)
			require.EqualExportedValues(t, tc.book, res)
		})
	}
}

func TestUpdateBook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "Success",
			err:  nil,
		},
		{
			name: "Error book not found",
			err:  entity.ErrBookNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, repo, _, _ := setupBookUsecase(t)

			book := entity.Book{
				ID:        "1",
				Name:      "name",
				AuthorIDs: []string{"2"},
			}

			repo.EXPECT().
				UpdateBook(context.Background(), book).
				Return(tc.err).
				Times(1)
			err := impl.UpdateBook(context.Background(), book.ID, book.Name, book.AuthorIDs)
			require.Equal(t, tc.err, err)
		})
	}
}

func TestGetAuthorBooks(t *testing.T) {
	t.Parallel()
	impl, repo, _, _ := setupBookUsecase(t)
	authorID := "1"
	books := []entity.Book{
		{
			ID:        "1",
			Name:      "name 1",
			AuthorIDs: []string{authorID},
		},
	}

	booksCh := make(chan entity.Book, 1)
	errCh := make(chan error, 1)
	booksCh <- books[0]
	close(errCh)
	close(booksCh)

	repo.EXPECT().
		GetAuthorBooks(context.Background(), authorID).
		Return(booksCh, errCh).
		Times(1)
	res, err := impl.GetAuthorBooks(context.Background(), authorID)

	received := make([]entity.Book, 0, 1)
	for b := range res {
		received = append(received, b)
	}
	require.ElementsMatch(t, books, received)
	receivedErr := <-err
	require.NoError(t, receivedErr)
}
