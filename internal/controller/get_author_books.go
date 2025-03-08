package controller

import (
	"github.com/rvekaterina/library/generated/api/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (i *implementation) GetAuthorBooks(req *library.GetAuthorBooksRequest, stream library.Library_GetAuthorBooksServer) error {
	i.logger.Info("get author books request", zap.String("id", req.GetAuthorId()))

	if err := req.ValidateAll(); err != nil {
		i.logger.Error("invalid get author's books request", zap.Error(err))
		return status.Error(codes.InvalidArgument, err.Error())
	}

	booksCh, errCh := i.bookUseCase.GetAuthorBooks(stream.Context(), req.GetAuthorId())

	for book := range booksCh {
		if err := stream.Send(&library.Book{
			Id:        book.ID,
			Name:      book.Name,
			AuthorId:  book.AuthorIDs,
			CreatedAt: timestamppb.New(book.CreatedAt),
			UpdatedAt: timestamppb.New(book.UpdatedAt),
		}); err != nil {
			i.logger.Error("failed to get author's books", zap.Error(err))
			return i.convertErr(err)
		}
	}
	if err := <-errCh; err != nil {
		i.logger.Error("failed to get author's books", zap.Error(err))
		return i.convertErr(err)
	}
	return nil
}
