package library

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/rvekaterina/library/internal/usecase/repository"

	"github.com/rvekaterina/library/internal/entity"
)

func (l *libraryImpl) RegisterBook(ctx context.Context, name string, authorIDs []string) (book entity.Book, err error) {
	err = l.transactor.WithTx(ctx, func(ctx context.Context) error {
		var txErr error

		book, txErr = l.bookRepository.CreateBook(ctx, entity.Book{
			Name:      name,
			AuthorIDs: authorIDs,
		})

		if txErr != nil {
			return txErr
		}
		serialized, txErr := json.Marshal(book)
		if txErr != nil {
			l.logger.Error("failed to serialize book", zap.Error(txErr))
			return txErr
		}

		idempotencyKey := repository.OutboxKindBook.String() + " " + book.ID

		txErr = l.outboxRepository.SendMessage(ctx, idempotencyKey, repository.OutboxKindBook, serialized)

		if txErr != nil {
			return txErr
		}
		return nil
	})

	if err != nil {
		return entity.Book{}, err
	}
	return book, err
}

func (l *libraryImpl) GetBook(ctx context.Context, bookID string) (entity.Book, error) {
	return l.bookRepository.GetBook(ctx, bookID)
}

func (l *libraryImpl) UpdateBook(ctx context.Context, id, name string, authorIDs []string) error {
	return l.bookRepository.UpdateBook(ctx, entity.Book{
		ID:        id,
		Name:      name,
		AuthorIDs: authorIDs,
	})
}

func (l *libraryImpl) GetAuthorBooks(ctx context.Context, authorID string) (<-chan entity.Book, <-chan error) {
	return l.bookRepository.GetAuthorBooks(ctx, authorID)
}
