package controller

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) GetBookInfo(ctx context.Context, req *library.GetBookInfoRequest) (*library.GetBookInfoResponse, error) {
	i.logger.Info("get book info request", zap.String("id", req.GetId()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid get book info request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	book, err := i.bookUseCase.GetBook(ctx, req.GetId())
	if err != nil {
		i.logger.Error("failed to get book", zap.Error(err))
		return nil, i.convertErr(err)
	}
	return &library.GetBookInfoResponse{
		Book: &library.Book{
			Id:        book.ID,
			Name:      book.Name,
			AuthorId:  book.AuthorIDs,
			CreatedAt: timestamppb.New(book.CreatedAt),
			UpdatedAt: timestamppb.New(book.UpdatedAt),
		},
	}, nil
}
