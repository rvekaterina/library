package controller

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) GetAuthorInfo(ctx context.Context, req *library.GetAuthorInfoRequest) (*library.GetAuthorInfoResponse, error) {
	i.logger.Info("get author info request", zap.String("id", req.GetId()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid get author info request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	author, err := i.authorUseCase.GetAuthor(ctx, req.GetId())
	if err != nil {
		i.logger.Error("get author info failed", zap.Error(err))
		return nil, i.convertErr(err)
	}
	return &library.GetAuthorInfoResponse{
		Id:        author.ID,
		Name:      author.Name,
		CreatedAt: timestamppb.New(author.CreatedAt),
		UpdatedAt: timestamppb.New(author.UpdatedAt),
	}, nil
}
