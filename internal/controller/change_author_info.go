package controller

import (
	"context"

	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) ChangeAuthorInfo(ctx context.Context, req *library.ChangeAuthorInfoRequest) (*library.ChangeAuthorInfoResponse, error) {
	i.logger.Info("change author info request",
		zap.String("id", req.GetId()),
		zap.String("name", req.GetName()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid change author info request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err := i.authorUseCase.ChangeAuthorInfo(ctx, req.GetId(), req.GetName())
	if err != nil {
		i.logger.Error("change author info request failed", zap.Error(err))
		return nil, i.convertErr(err)
	}
	return &library.ChangeAuthorInfoResponse{}, nil
}
