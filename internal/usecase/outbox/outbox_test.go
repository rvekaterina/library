package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rvekaterina/library/config"
	"github.com/rvekaterina/library/internal/usecase/repository"
	mocks "github.com/rvekaterina/library/internal/usecase/repository/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupMocks(t *testing.T) (*outboxImpl, *config.Config, *mocks.MockOutboxRepository, *mocks.MockTransactor) {
	t.Helper()
	logger := zap.NewNop()
	controller := gomock.NewController(t)
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	cfg := &config.Config{
		Outbox: config.Outbox{
			Enabled:     true,
			MaxAttempts: 3,
			RetryTTLMS:  time.Second,
			Mutex:       new(sync.RWMutex),
		},
	}

	globalHandler := func(kind repository.OutboxKind) (KindHandler, error) {
		return func(ctx context.Context, data []byte) error {
			return nil
		}, nil
	}

	o := New(logger, outboxRepo, globalHandler, cfg, transactor)
	return o, cfg, outboxRepo, transactor
}

func TestOutboxStops(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	o, _, _, transactor := setupMocks(t)
	workers := 1
	batchSize := 10
	waitTime := time.Millisecond * 100
	inProgressTTL := time.Millisecond * 50

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		Return(nil).
		Times(1)

	o.Start(ctx, workers, batchSize, waitTime, inProgressTTL)

	time.Sleep(waitTime / 2)

	cancel()

	time.Sleep(waitTime * 2)
}

func TestOutboxDisabled(t *testing.T) {
	t.Parallel()

	controller := gomock.NewController(t)
	logger := zap.NewNop()
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	cfg := &config.Config{
		Outbox: config.Outbox{
			Enabled: false,
		},
	}

	o := New(logger, outboxRepo, nil, cfg, transactor)

	ctx, cancel := context.WithCancel(context.Background())

	workers := 2
	batchSize := 10
	waitTime := time.Millisecond * 100
	inProgressTTL := time.Millisecond * 50

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		Return(nil).
		Times(0)

	o.Start(ctx, workers, batchSize, waitTime, inProgressTTL)

	time.Sleep(waitTime / 2)

	cancel()

	time.Sleep(waitTime * 2)
}

func TestOutboxProcessBatchSuccess(t *testing.T) {
	t.Parallel()
	o, cfg, outboxRepo, transactor := setupMocks(t)

	ctx := context.Background()

	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	testMessages := []repository.OutboxData{
		{
			IdempotencyKey: "key",
			Kind:           repository.OutboxKindBook,
			RawData:        []byte("test"),
		},
	}

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(ctx, batchSize, inProgressTTL, cfg.Outbox.MaxAttempts).
		Return(testMessages, nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkAsProcessed(ctx, []string{"key"}).
		Return(nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkForRetry(ctx, []string{}, cfg.Outbox.MaxAttempts, cfg.Outbox.RetryTTLMS).
		Return(nil).
		Times(1)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.NoError(t, err)
}

func TestProcessBatchWithHandlerError(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	logger := zap.NewNop()
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	cfg := &config.Config{
		Outbox: config.Outbox{
			Enabled:     true,
			MaxAttempts: 3,
			RetryTTLMS:  time.Second,
			Mutex:       new(sync.RWMutex),
		},
	}

	globalHandler := func(kind repository.OutboxKind) (KindHandler, error) {
		return func(ctx context.Context, data []byte) error {
			return ErrInternal
		}, nil
	}

	o := New(logger, outboxRepo, globalHandler, cfg, transactor)

	ctx := context.Background()

	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	testMessages := []repository.OutboxData{
		{
			IdempotencyKey: "key",
			Kind:           repository.OutboxKindAuthor,
			RawData:        []byte("test"),
		},
	}

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(ctx, batchSize, inProgressTTL, cfg.Outbox.MaxAttempts).
		Return(testMessages, nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkForRetry(ctx, []string{"key"}, cfg.Outbox.MaxAttempts, cfg.Outbox.RetryTTLMS).
		Return(nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkAsProcessed(ctx, []string{}).
		Return(nil).
		Times(1)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.NoError(t, err)
}

func TestProcessBatchWithTransactionError(t *testing.T) {
	t.Parallel()
	o, _, outboxRepo, transactor := setupMocks(t)
	ctx := context.Background()

	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		Return(errors.New("transaction error")).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.Error(t, err)
	require.ErrorContains(t, err, "transaction error")
}

func TestProcessBatchGetMessagesError(t *testing.T) {
	t.Parallel()
	o, cfg, outboxRepo, transactor := setupMocks(t)
	ctx := context.Background()

	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(ctx, batchSize, inProgressTTL, cfg.Outbox.MaxAttempts).
		Return(nil, errors.New("db error")).
		Times(1)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.Error(t, err)
	require.ErrorContains(t, err, "db error")
}

func TestProcessBatchWithMarkAsProcessedError(t *testing.T) {
	t.Parallel()
	o, cfg, outboxRepo, transactor := setupMocks(t)
	ctx := context.Background()

	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	testMessages := []repository.OutboxData{
		{
			IdempotencyKey: "key",
			Kind:           repository.OutboxKindBook,
			RawData:        []byte("test"),
		},
	}

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(ctx, batchSize, inProgressTTL, cfg.Outbox.MaxAttempts).
		Return(testMessages, nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkAsProcessed(ctx, []string{"key"}).
		Return(errors.New("mark as processed error")).
		Times(1)

	outboxRepo.EXPECT().
		MarkForRetry(ctx, []string{}, cfg.Outbox.MaxAttempts, cfg.Outbox.RetryTTLMS).
		Times(1)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.NoError(t, err)
}

func TestProcessBatchMarkForRetryError(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	logger := zap.NewNop()
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	cfg := &config.Config{
		Outbox: config.Outbox{
			Enabled:     true,
			MaxAttempts: 3,
			RetryTTLMS:  time.Second,
			Mutex:       new(sync.RWMutex),
		},
	}

	globalHandler := func(kind repository.OutboxKind) (KindHandler, error) {
		return func(ctx context.Context, data []byte) error {
			return ErrInternal
		}, nil
	}

	o := New(logger, outboxRepo, globalHandler, cfg, transactor)

	ctx := context.Background()
	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	testMessages := []repository.OutboxData{
		{
			IdempotencyKey: "key",
			Kind:           repository.OutboxKindAuthor,
			RawData:        []byte("test"),
		},
	}

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(ctx, batchSize, inProgressTTL, cfg.Outbox.MaxAttempts).
		Return(testMessages, nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkForRetry(ctx, []string{"key"}, cfg.Outbox.MaxAttempts, cfg.Outbox.RetryTTLMS).
		Return(errors.New("mark for retry error")).
		Times(1)

	outboxRepo.EXPECT().
		MarkAsProcessed(ctx, []string{}).
		Return(nil).
		Times(1)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.NoError(t, err)
}

func TestProcessBatchGetHandlerError(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	logger := zap.NewNop()
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	cfg := &config.Config{
		Outbox: config.Outbox{
			Enabled:     true,
			MaxAttempts: 3,
			RetryTTLMS:  time.Second,
			Mutex:       new(sync.RWMutex),
		},
	}

	globalHandler := func(kind repository.OutboxKind) (KindHandler, error) {
		return func(ctx context.Context, data []byte) error {
			return nil
		}, errors.New("get handler error")
	}

	o := New(logger, outboxRepo, globalHandler, cfg, transactor)

	ctx := context.Background()
	batchSize := 10
	inProgressTTL := time.Millisecond * 50

	testMessages := []repository.OutboxData{
		{
			IdempotencyKey: "key",
			Kind:           repository.OutboxKindAuthor,
			RawData:        []byte("test"),
		},
	}

	transactor.EXPECT().
		WithTx(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Times(1)

	outboxRepo.EXPECT().
		GetMessages(ctx, batchSize, inProgressTTL, cfg.Outbox.MaxAttempts).
		Return(testMessages, nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkForRetry(ctx, []string{}, cfg.Outbox.MaxAttempts, cfg.Outbox.RetryTTLMS).
		Return(nil).
		Times(1)

	outboxRepo.EXPECT().
		MarkAsProcessed(ctx, []string{}).
		Return(nil).
		Times(1)

	err := o.processBatch(ctx, batchSize, inProgressTTL)
	require.NoError(t, err)
}
