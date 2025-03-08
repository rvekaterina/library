package library

import (
	"context"

	"github.com/rvekaterina/library/internal/usecase/repository"
	"go.uber.org/zap"

	"github.com/rvekaterina/library/internal/entity"
)

//go:generate ../../../bin/mockgen -source=interfaces.go -destination=./mocks/usecase_mock.go -package=library
type (
	AuthorUseCase interface {
		RegisterAuthor(ctx context.Context, authorName string) (entity.Author, error)
		ChangeAuthorInfo(ctx context.Context, authorID, authorName string) error
		GetAuthor(ctx context.Context, authorID string) (entity.Author, error)
	}

	BookUseCase interface {
		RegisterBook(ctx context.Context, name string, authorIDs []string) (entity.Book, error)
		GetBook(ctx context.Context, bookID string) (entity.Book, error)
		UpdateBook(ctx context.Context, id, name string, authorIDs []string) error
		GetAuthorBooks(ctx context.Context, authorID string) (<-chan entity.Book, <-chan error)
	}
)

var _ AuthorUseCase = (*libraryImpl)(nil)
var _ BookUseCase = (*libraryImpl)(nil)

type libraryImpl struct {
	logger           *zap.Logger
	authorRepository repository.AuthorRepository
	bookRepository   repository.BookRepository
	outboxRepository repository.OutboxRepository
	transactor       repository.Transactor
}

func New(
	logger *zap.Logger,
	authorRepository repository.AuthorRepository,
	bookRepository repository.BookRepository,
	outboxRepository repository.OutboxRepository,
	transactor repository.Transactor,
) *libraryImpl {
	return &libraryImpl{
		logger:           logger,
		authorRepository: authorRepository,
		bookRepository:   bookRepository,
		outboxRepository: outboxRepository,
		transactor:       transactor,
	}
}
