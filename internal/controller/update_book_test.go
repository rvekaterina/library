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

func TestUpdateBook(t *testing.T) {
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
			name:     "Validation error book name",
			code:     codes.InvalidArgument,
			callTime: 0,
			book:     entity.Book{ID: uuid.NewString(), AuthorIDs: []string{uuid.NewString()}},
		},
		{
			name:     "Validation error book authorIDs",
			code:     codes.InvalidArgument,
			callTime: 0,
			book:     entity.Book{ID: uuid.NewString(), Name: "name", AuthorIDs: []string{"123a"}},
		},
		{
			name:     "Validation error bookID",
			code:     codes.InvalidArgument,
			callTime: 0,
			book:     entity.Book{ID: "1", Name: "name", AuthorIDs: []string{}},
		},
		{
			name:     "Book not found",
			code:     codes.NotFound,
			err:      entity.ErrBookNotFound,
			callTime: 1,
			book:     entity.Book{ID: uuid.NewString(), Name: "name", AuthorIDs: []string{}},
		},
		{
			name:     "Author not found",
			code:     codes.NotFound,
			err:      entity.ErrAuthorNotFound,
			callTime: 1,
			book:     entity.Book{ID: uuid.NewString(), Name: "name", AuthorIDs: []string{uuid.NewString()}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, bookUseCase := setupBookController(t)

			bookUseCase.EXPECT().
				UpdateBook(context.Background(), tc.book.ID, tc.book.Name, tc.book.AuthorIDs).
				Return(tc.err).
				Times(tc.callTime)

			resp, err := impl.UpdateBook(context.Background(), &library.UpdateBookRequest{
				Id:        tc.book.ID,
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
			}
		})
	}
}
