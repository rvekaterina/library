package controller

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
	"github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/entity"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetAuthorInfo(t *testing.T) {
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
			name:     "Validation error",
			code:     codes.InvalidArgument,
			callTime: 0,
			author:   entity.Author{},
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
			impl, bookUseCase := setupAuthorController(t)

			bookUseCase.EXPECT().
				GetAuthor(context.Background(), tc.author.ID).
				Return(tc.author, tc.err).
				Times(tc.callTime)

			resp, err := impl.GetAuthorInfo(context.Background(), &library.GetAuthorInfoRequest{Id: tc.author.ID})
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
				require.Equal(t, tc.author.ID, resp.GetId())
				require.Equal(t, tc.author.Name, resp.GetName())
				require.Equal(t, timestamppb.New(tc.author.CreatedAt), resp.GetCreatedAt())
				require.Equal(t, timestamppb.New(tc.author.UpdatedAt), resp.GetUpdatedAt())
			}
		})
	}
}
