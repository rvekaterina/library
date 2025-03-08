package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rvekaterina/library/internal/entity"
)

//go:generate ../../../bin/mockgen -source=interfaces.go -destination=./mocks/postgres_mock.go -package=repository
type (
	AuthorRepository interface {
		CreateAuthor(ctx context.Context, author entity.Author) (entity.Author, error)
		GetAuthor(ctx context.Context, id string) (entity.Author, error)
		ChangeAuthorInfo(ctx context.Context, author entity.Author) error
	}

	BookRepository interface {
		CreateBook(ctx context.Context, book entity.Book) (entity.Book, error)
		GetBook(ctx context.Context, bookID string) (entity.Book, error)
		UpdateBook(ctx context.Context, book entity.Book) error
		GetAuthorBooks(ctx context.Context, authorID string) (<-chan entity.Book, <-chan error)
	}

	OutboxRepository interface {
		SendMessage(ctx context.Context, idempotencyKey string, kind OutboxKind, message []byte) error
		GetMessages(ctx context.Context, batchSize int, inProgressTTL time.Duration, maxAttempt int) ([]OutboxData, error)
		MarkAsProcessed(ctx context.Context, idempotencyKeys []string) error
		MarkForRetry(ctx context.Context, idempotencyKeys []string, maxAttempt int, retryTTL time.Duration) error
	}

	pgxConn interface {
		Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
		QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
		Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
		Begin(ctx context.Context) (pgx.Tx, error)
		SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	}
)
