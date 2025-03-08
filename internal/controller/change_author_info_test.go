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

func TestChangeAuthorInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     codes.Code
		err      error
		callTime int
		author   entity.Author
	}{
		{
			name:     "Success",
			code:     codes.OK,
			callTime: 1,
			author:   entity.Author{ID: uuid.NewString(), Name: "name"},
		},
		{
			name:     "Validation error authorID",
			code:     codes.InvalidArgument,
			callTime: 0,
			author:   entity.Author{ID: "123", Name: "name"},
		},
		{
			name:     "Validation error author name",
			code:     codes.InvalidArgument,
			callTime: 0,
			author:   entity.Author{ID: uuid.NewString(), Name: "((!!13))"},
		},
		{
			name:     "Validation error invalid name length",
			code:     codes.InvalidArgument,
			callTime: 0,
			author:   entity.Author{ID: uuid.NewString()},
		},
		{
			name:     "Author not found",
			code:     codes.NotFound,
			err:      entity.ErrAuthorNotFound,
			callTime: 1,
			author:   entity.Author{ID: uuid.NewString(), Name: "name"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, authorUseCase := setupAuthorController(t)

			authorUseCase.EXPECT().
				ChangeAuthorInfo(context.Background(), tc.author.ID, tc.author.Name).
				Return(tc.err).
				Times(tc.callTime)

			resp, err := impl.ChangeAuthorInfo(context.Background(), &library.ChangeAuthorInfoRequest{
				Id:   tc.author.ID,
				Name: tc.author.Name,
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
