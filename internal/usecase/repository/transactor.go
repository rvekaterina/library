package repository

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/jackc/pgx/v5"
)

//go:generate ../../../bin/mockgen -source=transactor.go -destination=./mocks/transactor_mock.go -package=repository
type Transactor interface {
	WithTx(ctx context.Context, function func(ctx context.Context) error) error
}

var _ Transactor = (*transactorImpl)(nil)

type transactorImpl struct {
	db     pgxConn
	logger *zap.Logger
}

func NewTransactor(db pgxConn, logger *zap.Logger) *transactorImpl {
	return &transactorImpl{
		db:     db,
		logger: logger,
	}
}

func (t *transactorImpl) WithTx(ctx context.Context, function func(ctx context.Context) error) (txErr error) {
	ctxWithTx, tx, err := injectTx(ctx, t.db)
	if err != nil {
		return fmt.Errorf("cannot inject transaction: %w", err)
	}

	defer func() {
		if txErr != nil {
			err = tx.Rollback(ctxWithTx)
			if err != nil {
				t.logger.Debug("cannot rollback transaction", zap.Error(err))
			}
			return
		}
		txErr = tx.Commit(ctxWithTx)
		if txErr != nil {
			t.logger.Error("cannot commit transaction", zap.Error(txErr))
		}
	}()

	err = function(ctxWithTx)
	if err != nil {
		return err
	}

	return nil
}

var ErrTxNotFound = errors.New("cannot extract tx from context")

func extractTx(ctx context.Context) (pgx.Tx, error) {
	tx, ok := ctx.Value(txInjector{}).(pgx.Tx)
	if !ok {
		return tx, ErrTxNotFound
	}
	return tx, nil
}

type txInjector struct {
}

func injectTx(ctx context.Context, pool pgxConn) (context.Context, pgx.Tx, error) {
	if tx, err := extractTx(ctx); err == nil {
		return ctx, tx, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(ctx, txInjector{}, tx), tx, nil
}
