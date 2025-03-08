package controller

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/entity"
	mocks "github.com/rvekaterina/library/internal/usecase/library/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupBookController(t *testing.T) (*implementation, *mocks.MockBookUseCase) {
	t.Helper()
	controller := gomock.NewController(t)
	mockBookUseCase := mocks.NewMockBookUseCase(controller)
	logger := zap.NewNop()
	impl := New(logger, mockBookUseCase, nil)
	return impl, mockBookUseCase
}

func TestAddBook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     codes.Code
		err      error
		callTime int
		book     entity.Book
	}{
		{
			name:     "Success",
			code:     codes.OK,
			callTime: 1,
			book:     entity.Book{Name: "name", AuthorIDs: []string{}},
		},
		{
			name:     "Validation error book name",
			code:     codes.InvalidArgument,
			callTime: 0,
			book:     entity.Book{AuthorIDs: []string{uuid.NewString()}},
		},
		{
			name:     "Validation error book authorIDs",
			code:     codes.InvalidArgument,
			callTime: 0,
			book:     entity.Book{Name: "book name", AuthorIDs: []string{"12345"}},
		},
		{
			name:     "Already exists",
			code:     codes.AlreadyExists,
			err:      entity.ErrBookAlreadyExists,
			callTime: 1,
			book:     entity.Book{Name: "name", AuthorIDs: []string{}},
		},
		{
			name:     "Author not found",
			code:     codes.NotFound,
			err:      entity.ErrAuthorNotFound,
			callTime: 1,
			book:     entity.Book{Name: "name", AuthorIDs: []string{uuid.NewString()}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, bookUseCase := setupBookController(t)
			bookUseCase.EXPECT().
				RegisterBook(context.Background(), tc.book.Name, tc.book.AuthorIDs).
				Return(tc.book, tc.err).
				Times(tc.callTime)

			resp, err := impl.AddBook(context.Background(), &library.AddBookRequest{
				Name:      tc.book.Name,
				AuthorIds: tc.book.AuthorIDs,
			})

			require.Equal(t, tc.code, status.Code(err))
			if tc.code != codes.OK {
				require.Nil(t, resp)
				require.Error(t, err)
				if tc.code != codes.InvalidArgument {
					require.Contains(t, err.Error(), tc.err.Error())
				}
			} else {
				require.NotNil(t, resp)
				require.NoError(t, err)
				require.Equal(t, tc.book.Name, resp.GetBook().GetName())
				require.ElementsMatch(t, tc.book.AuthorIDs, resp.GetBook().GetAuthorId())
			}
		})
	}
}
