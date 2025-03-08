package controller

import (
	generated "github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/usecase/library"
	"go.uber.org/zap"
)

var _ generated.LibraryServer = (*implementation)(nil)

type implementation struct {
	logger        *zap.Logger
	bookUseCase   library.BookUseCase
	authorUseCase library.AuthorUseCase
}

func New(
	logger *zap.Logger,
	bookUseCase library.BookUseCase,
	authorUseCase library.AuthorUseCase,
) *implementation {
	return &implementation{
		logger:        logger,
		bookUseCase:   bookUseCase,
		authorUseCase: authorUseCase,
	}
}
