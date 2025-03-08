package controller

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) RegisterAuthor(ctx context.Context, req *library.RegisterAuthorRequest) (*library.RegisterAuthorResponse, error) {
	i.logger.Info("register author request", zap.String("name", req.GetName()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid register author request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	author, err := i.authorUseCase.RegisterAuthor(ctx, req.GetName())
	if err != nil {
		i.logger.Error("register author request failed", zap.Error(err))
		return nil, i.convertErr(err)
	}
	return &library.RegisterAuthorResponse{
		Id:        author.ID,
		CreatedAt: timestamppb.New(author.CreatedAt),
		UpdatedAt: timestamppb.New(author.UpdatedAt),
	}, nil
}
