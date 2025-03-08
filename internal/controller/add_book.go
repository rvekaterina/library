package controller

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) AddBook(ctx context.Context, req *library.AddBookRequest) (*library.AddBookResponse, error) {
	i.logger.Info("add book request",
		zap.String("name", req.GetName()),
		zap.Strings("author_ids", req.GetAuthorIds()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid add book request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	book, err := i.bookUseCase.RegisterBook(ctx, req.GetName(), req.GetAuthorIds())
	if err != nil {
		i.logger.Error("add book failed", zap.Error(err))
		return nil, i.convertErr(err)
	}
	return &library.AddBookResponse{
		Book: &library.Book{
			Id:        book.ID,
			Name:      book.Name,
			AuthorId:  book.AuthorIDs,
			CreatedAt: timestamppb.New(book.CreatedAt),
			UpdatedAt: timestamppb.New(book.UpdatedAt),
		},
	}, nil
}
