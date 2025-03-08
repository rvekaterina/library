package controller

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	generated "github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/entity"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockStream struct {
	generated.Library_GetAuthorBooksServer
	books []*generated.Book
	err   bool
	ctx   context.Context
}

func newStream(ctx context.Context, err bool) *mockStream {
	return &mockStream{
		books: make([]*generated.Book, 0),
		err:   err,
		ctx:   ctx,
	}
}

func (m *mockStream) Context() context.Context {
	return m.ctx
}

func (m *mockStream) Send(book *generated.Book) error {
	if m.err {
		return errors.New("stream sending error")
	}
	m.books = append(m.books, book)
	return nil
}

func TestGetAuthorBooksSuccess(t *testing.T) {
	t.Parallel()
	impl, bookUseCase := setupBookController(t)
	stream := newStream(context.Background(), false)

	authorID := uuid.NewString()

	book := entity.Book{
		ID:        uuid.NewString(),
		Name:      "name",
		AuthorIDs: []string{authorID},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	booksCh := make(chan entity.Book)
	errCh := make(chan error, 1)
	go func() {
		defer close(booksCh)
		defer close(errCh)
		booksCh <- book
	}()

	bookUseCase.EXPECT().
		GetAuthorBooks(context.Background(), authorID).
		Return(booksCh, errCh).
		Times(1)

	err := impl.GetAuthorBooks(&generated.GetAuthorBooksRequest{AuthorId: authorID}, stream)
	require.NoError(t, err)
	require.Equal(t, codes.OK, status.Code(err))
	require.Len(t, stream.books, 1)
	require.Equal(t, book.ID, stream.books[0].GetId())
	require.ElementsMatch(t, book.AuthorIDs, stream.books[0].GetAuthorId())
	require.Equal(t, book.Name, stream.books[0].GetName())
	require.Equal(t, timestamppb.New(book.CreatedAt), stream.books[0].GetCreatedAt())
	require.Equal(t, timestamppb.New(book.UpdatedAt), stream.books[0].GetUpdatedAt())
}

func TestGetAuthorBooksError(t *testing.T) {
	t.Parallel()

	const errorWhileSending = "Error while sending"

	tests := []struct {
		name     string
		code     codes.Code
		authorID string
		book     entity.Book
	}{
		{
			name:     "Validation error",
			authorID: "1",
			code:     codes.InvalidArgument,
		},
		{
			name:     errorWhileSending,
			code:     codes.Internal,
			authorID: uuid.NewString(),
			book: entity.Book{
				ID:        uuid.NewString(),
				Name:      "name",
				AuthorIDs: []string{"1"},
			},
		},
		{
			name:     "Error in error channel",
			code:     codes.Internal,
			authorID: uuid.NewString(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, bookUseCase := setupBookController(t)

			var (
				err     error
				stream  *mockStream
				booksCh = make(chan entity.Book)
				errCh   = make(chan error, 1)
			)

			switch tc.code {
			case codes.InvalidArgument:
				err = impl.GetAuthorBooks(&generated.GetAuthorBooksRequest{AuthorId: tc.authorID}, nil)
			case codes.Internal:
				if tc.name == errorWhileSending {
					stream = newStream(context.Background(), true)
					go func() {
						defer close(booksCh)
						defer close(errCh)
						booksCh <- tc.book
					}()
				} else {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					stream = newStream(ctx, false)
					go func() {
						defer close(booksCh)
						defer close(errCh)
						errCh <- ctx.Err()
					}()
				}

				bookUseCase.EXPECT().
					GetAuthorBooks(gomock.Any(), tc.authorID).
					Return(booksCh, errCh).
					Times(1)

				err = impl.GetAuthorBooks(&generated.GetAuthorBooksRequest{AuthorId: tc.authorID}, stream)

				require.Empty(t, stream.books)
			}

			require.Error(t, err)
			require.Equal(t, tc.code, status.Code(err))
		})
	}
}
