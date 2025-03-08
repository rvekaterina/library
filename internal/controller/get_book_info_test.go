package controller

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/entity"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetBookInfo(t *testing.T) {
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
			book:     entity.Book{ID: uuid.NewString(), Name: "name", AuthorIDs: []string{}},
		},
		{
			name:     "Validation error",
			code:     codes.InvalidArgument,
			callTime: 0,
			book:     entity.Book{ID: "1"},
		},
		{
			name:     "Book not found",
			code:     codes.NotFound,
			err:      entity.ErrBookNotFound,
			callTime: 1,
			book:     entity.Book{ID: uuid.NewString(), Name: "name", AuthorIDs: []string{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, bookUseCase := setupBookController(t)

			bookUseCase.EXPECT().
				GetBook(context.Background(), tc.book.ID).
				Return(tc.book, tc.err).
				Times(tc.callTime)

			resp, err := impl.GetBookInfo(context.Background(), &library.GetBookInfoRequest{Id: tc.book.ID})
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
				require.Equal(t, tc.book.ID, resp.GetBook().GetId())
				require.Equal(t, tc.book.Name, resp.GetBook().GetName())
				require.ElementsMatch(t, tc.book.AuthorIDs, resp.GetBook().GetAuthorId())
			}
		})
	}
}
