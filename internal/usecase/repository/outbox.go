package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"go.uber.org/zap"

	"github.com/jackc/pgx/v5"
)

type OutboxKind int

const (
	OutboxKindAuthor OutboxKind = iota
	OutboxKindBook
)

func (o OutboxKind) String() string {
	switch o {
	case OutboxKindAuthor:
		return "author"
	case OutboxKindBook:
		return "book"
	default:
		return "undefined"
	}
}

type OutboxData struct {
	IdempotencyKey string
	Kind           OutboxKind
	RawData        []byte
}

var _ OutboxRepository = (*outboxRepository)(nil)

type outboxRepository struct {
	db     pgxConn
	logger *zap.Logger
}

func NewOutbox(db pgxConn, logger *zap.Logger) *outboxRepository {
	return &outboxRepository{
		db:     db,
		logger: logger,
	}
}

func (o *outboxRepository) SendMessage(ctx context.Context, idempotencyKey string, kind OutboxKind, message []byte) error {
	const query = `
INSERT INTO outbox (idempotency_key, data, status, kind)
VALUES ($1, $2, 'CREATED', $3)
ON CONFLICT (idempotency_key) DO NOTHING
`
	o.logger.Debug("sending message to outbox repository", zap.String("idempotency_key", idempotencyKey), zap.Int("kind", int(kind)),
		zap.ByteString("message", message))
	var err error
	if tx, txErr := extractTx(ctx); txErr == nil {
		_, err = tx.Exec(ctx, query, idempotencyKey, message, kind)
	} else {
		_, err = o.db.Exec(ctx, query, idempotencyKey, message, kind)
	}

	if err != nil {
		o.logger.Debug("failed to send message to outbox repository", zap.Error(err), zap.String("idempotencyKey", idempotencyKey))
		return errors.Wrap(err, `failed to send message to outbox repository`)
	}
	return nil
}

func (o *outboxRepository) GetMessages(ctx context.Context, batchSize int, inProgressTTL time.Duration, maxAttempt int) ([]OutboxData, error) {
	const query = `
UPDATE outbox
SET status = 'IN_PROGRESS'
WHERE idempotency_key IN (
    SELECT idempotency_key
    FROM outbox
    WHERE ((status = 'CREATED' OR (status = 'IN_PROGRESS' AND updated_at < now() - $1::interval) OR status = 'RETRY')
    AND (retry_at IS NULL OR retry_at <= now())
    AND attempt < $3)
    ORDER BY created_at
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
RETURNING idempotency_key, data, kind
`
	timeInterval := strconv.FormatInt(inProgressTTL.Milliseconds(), 10) + " ms"
	o.logger.Debug("trying to get messages from outbox repository", zap.String("in_progress_ttl", timeInterval),
		zap.Int("batch_size", batchSize), zap.Int("max_attempts", maxAttempt))

	var (
		err  error
		rows pgx.Rows
	)
	if tx, txErr := extractTx(ctx); txErr == nil {
		rows, err = tx.Query(ctx, query, timeInterval, batchSize, maxAttempt)
	} else {
		rows, err = o.db.Query(ctx, query, timeInterval, batchSize, maxAttempt)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get messages from outbox repository")
	}
	defer rows.Close()
	res := make([]OutboxData, 0, batchSize)

	for rows.Next() {
		var idempotencyKey string
		var data []byte
		var kind OutboxKind

		if err = rows.Scan(&idempotencyKey, &data, &kind); err != nil {
			return nil, errors.Wrap(err, "scan error in outbox repository: get messages")
		}
		res = append(res, OutboxData{
			IdempotencyKey: idempotencyKey,
			RawData:        data,
			Kind:           kind,
		})
	}
	return res, rows.Err()
}

func (o *outboxRepository) MarkAsProcessed(ctx context.Context, idempotencyKeys []string) error {
	if len(idempotencyKeys) == 0 {
		return nil
	}

	const query = `
UPDATE outbox
SET status = 'SUCCESS', retry_at = NULL
WHERE idempotency_key = ANY($1)
`
	o.logger.Debug("marking keys as processed in outbox repository", zap.Strings("idempotency_keys", idempotencyKeys))

	var err error
	if tx, txErr := extractTx(ctx); txErr == nil {
		_, err = tx.Exec(ctx, query, idempotencyKeys)
	} else {
		_, err = o.db.Exec(ctx, query, idempotencyKeys)
	}

	return errors.Wrap(err, "failed to mark keys as processed in outbox repository")
}

func (o *outboxRepository) MarkForRetry(ctx context.Context, idempotencyKeys []string, maxAttempt int, retryTTL time.Duration) error {
	if len(idempotencyKeys) == 0 {
		return nil
	}

	const query = `
UPDATE outbox
SET status   = CASE WHEN attempt >= $2 THEN 'FAILED' ELSE 'RETRY' END,
    retry_at = CASE
                   WHEN attempt >= $2 THEN NULL
                   ELSE now() + $3::interval
        END,
	attempt = attempt + 1
    WHERE idempotency_key = ANY($1)
`

	o.logger.Debug("marking keys for retry in outbox repository", zap.Strings("idempotency_keys", idempotencyKeys),
		zap.Int("max_attempts", maxAttempt), zap.String("retry_ttl", retryTTL.String()))

	timeInterval := strconv.FormatInt(retryTTL.Milliseconds(), 10) + " ms"

	var err error
	if tx, txErr := extractTx(ctx); txErr == nil {
		_, err = tx.Exec(ctx, query, idempotencyKeys, maxAttempt, timeInterval)
	} else {
		_, err = o.db.Exec(ctx, query, idempotencyKeys, maxAttempt, timeInterval)
	}

	return errors.Wrap(err, "failed to mark keys as created in outbox repository")
}
