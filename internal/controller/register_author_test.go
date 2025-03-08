package controller

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/golang/mock/gomock"
	"github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/entity"
	mocks "github.com/rvekaterina/library/internal/usecase/library/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupAuthorController(t *testing.T) (*implementation, *mocks.MockAuthorUseCase) {
	t.Helper()
	controller := gomock.NewController(t)
	mockAuthorUseCase := mocks.NewMockAuthorUseCase(controller)
	logger := zap.NewNop()
	impl := New(logger, nil, mockAuthorUseCase)
	return impl, mockAuthorUseCase
}

func TestRegisterAuthor(t *testing.T) {
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
			author:   entity.Author{Name: "name"},
		},
		{
			name:     "Validation error invalid name length",
			code:     codes.InvalidArgument,
			callTime: 0,
			author:   entity.Author{},
		},
		{
			name:     "Validation error name pattern",
			code:     codes.InvalidArgument,
			callTime: 0,
			author:   entity.Author{Name: " !+ )( "},
		},
		{
			name:     "Already exists",
			code:     codes.AlreadyExists,
			err:      entity.ErrAuthorAlreadyExists,
			callTime: 1,
			author:   entity.Author{Name: "name"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, authorUseCase := setupAuthorController(t)

			authorUseCase.EXPECT().
				RegisterAuthor(context.Background(), tc.author.Name).
				Return(tc.author, tc.err).
				Times(tc.callTime)

			resp, err := impl.RegisterAuthor(context.Background(), &library.RegisterAuthorRequest{
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
				require.Equal(t, tc.author.ID, resp.GetId())
				require.Equal(t, timestamppb.New(tc.author.CreatedAt), resp.GetCreatedAt())
				require.Equal(t, timestamppb.New(tc.author.UpdatedAt), resp.GetUpdatedAt())
			}
		})
	}
}
