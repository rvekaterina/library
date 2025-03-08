package controller

import (
	"context"

	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) UpdateBook(ctx context.Context, req *library.UpdateBookRequest) (*library.UpdateBookResponse, error) {
	i.logger.Info("update book request", zap.String("id", req.GetId()),
		zap.String("name", req.GetName()),
		zap.Strings("author_ids", req.GetAuthorIds()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid update book request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err := i.bookUseCase.UpdateBook(ctx, req.GetId(), req.GetName(), req.GetAuthorIds())
	if err != nil {
		i.logger.Error("failed to update book", zap.Error(err))
		return nil, i.convertErr(err)
	}
	return &library.UpdateBookResponse{}, nil
}
