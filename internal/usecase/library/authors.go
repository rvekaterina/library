package library

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/rvekaterina/library/internal/usecase/repository"

	"github.com/rvekaterina/library/internal/entity"
)

func (l *libraryImpl) RegisterAuthor(ctx context.Context, authorName string) (author entity.Author, err error) {
	err = l.transactor.WithTx(ctx, func(ctx context.Context) error {
		var txErr error

		author, txErr = l.authorRepository.CreateAuthor(ctx, entity.Author{
			Name: authorName,
		})

		if txErr != nil {
			return txErr
		}
		serialized, txErr := json.Marshal(author)
		if txErr != nil {
			l.logger.Error("failed to serialize author", zap.Error(txErr))
			return txErr
		}

		idempotencyKey := repository.OutboxKindAuthor.String() + " " + author.ID

		txErr = l.outboxRepository.SendMessage(ctx, idempotencyKey, repository.OutboxKindAuthor, serialized)

		if txErr != nil {
			return txErr
		}
		return nil
	})

	if err != nil {
		return entity.Author{}, err
	}

	return author, err
}

func (l *libraryImpl) ChangeAuthorInfo(ctx context.Context, authorID, authorName string) error {
	return l.authorRepository.ChangeAuthorInfo(ctx, entity.Author{
		ID:   authorID,
		Name: authorName,
	})
}

func (l *libraryImpl) GetAuthor(ctx context.Context, authorID string) (entity.Author, error) {
	return l.authorRepository.GetAuthor(ctx, authorID)
}
