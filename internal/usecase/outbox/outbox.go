package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/rvekaterina/library/config"
	"github.com/rvekaterina/library/internal/usecase/repository"
	"go.uber.org/zap"
)

var (
	ErrInternal = errors.New("internal server error")
)

type GlobalHandler = func(kind repository.OutboxKind) (KindHandler, error)

type KindHandler = func(ctx context.Context, data []byte) error

type Outbox interface {
	Start(ctx context.Context, workers, batchSize int, waitTime time.Duration, inProgressTTLSeconds time.Duration)
}

var _ Outbox = (*outboxImpl)(nil)

type outboxImpl struct {
	logger           *zap.Logger
	outboxRepository repository.OutboxRepository
	globalHandler    GlobalHandler
	cfg              *config.Config
	transactor       repository.Transactor
}

func New(
	logger *zap.Logger,
	outboxRepository repository.OutboxRepository,
	globalHandler GlobalHandler,
	cfg *config.Config,
	transactor repository.Transactor,
) *outboxImpl {
	return &outboxImpl{
		logger:           logger,
		outboxRepository: outboxRepository,
		globalHandler:    globalHandler,
		cfg:              cfg,
		transactor:       transactor,
	}
}

func (o *outboxImpl) Start(
	ctx context.Context,
	workers,
	batchSize int,
	waitTime time.Duration,
	inProgressTTLMS time.Duration,
) {
	wg := new(sync.WaitGroup)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go o.worker(ctx, wg, batchSize, waitTime, inProgressTTLMS)
	}
	go func() {
		wg.Wait()
	}()
}

func (o *outboxImpl) worker(
	ctx context.Context,
	wg *sync.WaitGroup,
	batchSize int,
	waitTime time.Duration,
	inProgressTTLMS time.Duration,
) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			o.logger.Info("worker stopped")
			return
		default:
			time.Sleep(waitTime)
		}

		if !o.cfg.Outbox.Enabled {
			continue
		}

		err := o.processBatch(ctx, batchSize, inProgressTTLMS)
		if err != nil {
			o.logger.Error("outbox worker error", zap.Error(err))
		}
	}
}

func (o *outboxImpl) processBatch(
	ctx context.Context,
	batchSize int,
	inProgressTTLMS time.Duration,
) error {
	return o.transactor.WithTx(ctx, func(ctx context.Context) error {
		o.cfg.Outbox.Mutex.RLock()
		var (
			maxAttempts = o.cfg.Outbox.MaxAttempts
			retryTTL    = o.cfg.Outbox.RetryTTLMS
		)
		o.cfg.Outbox.Mutex.RUnlock()

		messages, err := o.outboxRepository.GetMessages(ctx, batchSize, inProgressTTLMS, maxAttempts)
		if err != nil {
			o.logger.Debug("failed to get messages from outbox", zap.Error(err))
			return err
		}

		o.logger.Info("got messages from outbox", zap.Int("messages", len(messages)))

		successKeys := make([]string, 0, len(messages))
		failedKeys := make([]string, 0, len(messages))

		for _, message := range messages {
			key := message.IdempotencyKey
			kindHandler, errHandler := o.globalHandler(message.Kind)

			if errHandler != nil {
				o.logger.Error("failed to get kind handler", zap.Error(err))
				continue
			}

			err = kindHandler(ctx, message.RawData)
			if errors.Is(err, ErrInternal) {
				failedKeys = append(failedKeys, key)
				o.logger.Debug("error while handling", zap.Error(err), zap.Strings("failed_keys", failedKeys))
				continue
			}
			successKeys = append(successKeys, key)
		}

		err = o.outboxRepository.MarkAsProcessed(ctx, successKeys)
		if err != nil {
			o.logger.Debug("failed to mark keys as processed in outbox repository", zap.Error(err))
		}

		err = o.outboxRepository.MarkForRetry(ctx, failedKeys, maxAttempts, retryTTL)
		if err != nil {
			o.logger.Debug("failed to mark failed keys for retry in outbox repository", zap.Error(err))
		}
		return nil
	})
}
