package repository

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
	"github.com/rvekaterina/library/internal/entity"
)

var (
	_ BookRepository   = (*postgresRepository)(nil)
	_ AuthorRepository = (*postgresRepository)(nil)
)

const foreignKeyViolationCode = "23503"

type postgresRepository struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func New(db *pgxpool.Pool, logger *zap.Logger) *postgresRepository {
	return &postgresRepository{
		db:     db,
		logger: logger,
	}
}

func (p *postgresRepository) CreateAuthor(ctx context.Context, author entity.Author) (res entity.Author, txErr error) {
	const query = `
INSERT INTO author (name)
VALUES ($1)
RETURNING id, created_at, updated_at
`
	p.logger.Debug("create author query", zap.String("name", author.Name))

	var (
		tx  pgxConn
		err error
	)

	if tx, err = extractTx(ctx); err != nil {
		tx = p.db
	}

	err = tx.QueryRow(ctx, query, author.Name).Scan(&author.ID, &author.CreatedAt, &author.UpdatedAt)
	if err != nil {
		return entity.Author{}, errors.Wrap(err, "scan error")
	}

	p.logger.Debug("author created successfully", zap.String("id", author.ID))
	return author, nil
}

func (p *postgresRepository) GetAuthor(ctx context.Context, id string) (entity.Author, error) {
	p.logger.Debug("fetching author", zap.String("id", id))

	const query = `
SELECT name, created_at, updated_at
FROM author
WHERE id = $1
`
	var (
		tx  pgxConn
		err error
	)

	if tx, err = extractTx(ctx); err != nil {
		tx = p.db
	}

	author := entity.Author{ID: id}
	err = tx.QueryRow(ctx, query, id).Scan(&author.Name, &author.CreatedAt, &author.UpdatedAt)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return entity.Author{}, entity.ErrAuthorNotFound
	case err != nil:
		return entity.Author{}, errors.Wrap(err, "scan error")
	}

	p.logger.Debug("author fetched successfully", zap.String("id", id))
	return author, nil
}

func (p *postgresRepository) ChangeAuthorInfo(ctx context.Context, author entity.Author) error {
	p.logger.Debug("updating author info", zap.String("author_id", author.ID), zap.String("name", author.Name))
	const query = `
UPDATE author
SET name = $2
WHERE id = $1
`
	var (
		tx  pgxConn
		err error
	)

	if tx, err = extractTx(ctx); err != nil {
		tx = p.db
	}

	_, err = tx.Exec(ctx, query, author.ID, author.Name)
	if err != nil {
		return fmt.Errorf("failed to update author id=%s, name=%s: %w", author.ID, author.Name, err)
	}

	p.logger.Debug("author info updated successfully", zap.String("author_id", author.ID))
	return nil
}

func (p *postgresRepository) CreateBook(ctx context.Context, book entity.Book) (res entity.Book, txErr error) {
	p.logger.Debug("starting create book transaction", zap.String("book_name", book.Name))

	var (
		tx  pgx.Tx
		err error
	)

	if tx, err = extractTx(ctx); err != nil {
		tx, err = p.db.Begin(ctx)
		if err == nil {
			defer func(tx pgx.Tx, ctx context.Context) {
				if txErr != nil {
					p.rollbackErrorHandle(ctx, tx)
					return
				}
				if txErr = tx.Commit(ctx); txErr != nil {
					p.logger.Error("failed to commit transaction", zap.Error(txErr))
				}
			}(tx, ctx)
		}
	}

	if err != nil {
		return entity.Book{}, errors.Wrap(err, "begin transaction error")
	}

	const query = `
INSERT INTO book (name)
VALUES ($1)
RETURNING id, created_at, updated_at
`
	err = tx.QueryRow(ctx, query, book.Name).Scan(&book.ID, &book.CreatedAt, &book.UpdatedAt)
	if err != nil {
		return entity.Book{}, errors.Wrap(err, "scan error")
	}

	if err = p.insertBatch(ctx, tx, book); err != nil {
		return entity.Book{}, err
	}

	p.logger.Debug("book created successfully", zap.String("book_id", book.ID))
	return book, nil
}

func (p *postgresRepository) GetBook(ctx context.Context, bookID string) (entity.Book, error) {
	p.logger.Debug("fetching book", zap.String("book_id", bookID))

	const query = `
SELECT b.name,
       b.created_at,
       b.updated_at,
       ARRAY_REMOVE(ARRAY_AGG(ab.author_id), NULL) AS author_ids
FROM book b
         LEFT JOIN author_book ab ON ab.book_id = b.id
WHERE b.id = $1
GROUP BY b.id, b.name, b.created_at, b.updated_at;
`

	var (
		tx  pgxConn
		err error
	)

	if tx, err = extractTx(ctx); err != nil {
		tx = p.db
	}

	book := entity.Book{ID: bookID}
	err = tx.QueryRow(ctx, query, bookID).Scan(&book.Name, &book.CreatedAt, &book.UpdatedAt, &book.AuthorIDs)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return entity.Book{}, entity.ErrBookNotFound
	case err != nil:
		return entity.Book{}, errors.Wrap(err, "scan error")
	}

	p.logger.Debug("book fetched successfully", zap.String("book_id", bookID))
	return book, nil
}

func (p *postgresRepository) UpdateBook(ctx context.Context, book entity.Book) (txErr error) {
	p.logger.Debug("starting book update transaction", zap.String("book_id", book.ID))

	var (
		tx  pgx.Tx
		err error
	)

	if tx, err = extractTx(ctx); err != nil {
		tx, err = p.db.Begin(ctx)
		if err == nil {
			defer func(tx pgx.Tx, ctx context.Context) {
				if txErr != nil {
					p.rollbackErrorHandle(ctx, tx)
					return
				}
				if txErr = tx.Commit(ctx); txErr != nil {
					p.logger.Error("failed to commit transaction", zap.Error(txErr))
				}
			}(tx, ctx)
		}
	}

	if err != nil {
		return errors.Wrap(err, "begin transaction error")
	}

	const query = `
UPDATE book
SET name = $2
WHERE id = $1
`
	_, err = tx.Exec(ctx, query, book.ID, book.Name)
	if err != nil {
		return err
	}

	const queryDelete = `
DELETE
FROM author_book
WHERE book_id = $1
`

	_, err = tx.Exec(ctx, queryDelete, book.ID)
	if err != nil {
		return err
	}

	if err = p.insertBatch(ctx, tx, book); err != nil {
		return err
	}

	p.logger.Debug("book updated successfully", zap.String("book_id", book.ID))
	return nil
}

func (p *postgresRepository) GetAuthorBooks(ctx context.Context, authorID string) (<-chan entity.Book, <-chan error) {
	p.logger.Debug("fetching books for author", zap.String("author_id", authorID))

	const query = `
DECLARE author_books_cursor CURSOR FOR
SELECT b.id,
       b.name,
       b.created_at,
       b.updated_at,
       ARRAY_AGG(ab.author_id) AS author_ids
FROM book b
LEFT JOIN author_book ab ON ab.book_id = b.id
WHERE b.id IN (SELECT ab.book_id
                  FROM author_book ab
                  WHERE ab.author_id = $1)
GROUP BY b.id, b.name, b.created_at, b.updated_at
`
	booksCh := make(chan entity.Book)
	errCh := make(chan error, 1)

	go func() {
		defer close(booksCh)
		defer close(errCh)

		var (
			tx  pgx.Tx
			err error
			// txErr error
		)

		if tx, err = extractTx(ctx); err != nil {
			tx, err = p.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
			if err == nil {
				defer func(tx pgx.Tx, ctx context.Context) {
					if err != nil {
						p.rollbackErrorHandle(ctx, tx)
						return
					}
					if err = tx.Commit(ctx); err != nil {
						errCh <- errors.Wrap(err, "commit transaction error")
						p.logger.Error("failed to commit transaction", zap.Error(err))
					}
				}(tx, ctx)
			}
		}

		if err != nil {
			errCh <- errors.Wrap(err, "begin transaction error")
		}

		if _, err = tx.Exec(ctx, query, authorID); err != nil {
			errCh <- errors.Wrap(err, "declare cursor error")
			return
		}

		const batchSize = 100

		defer func() {
			if _, err = tx.Exec(ctx, `CLOSE author_books_cursor`); err != nil {
				p.logger.Debug("failed to close cursor", zap.Error(err))
			}
		}()

		for {
			hasData, batchErr := p.processBatch(ctx, tx, batchSize, booksCh)
			if batchErr != nil {
				err = batchErr
				errCh <- batchErr
				return
			}
			if !hasData {
				break
			}
		}

		p.logger.Debug("fetched books for author", zap.String("author_id", authorID))
	}()

	return booksCh, errCh
}

func (p *postgresRepository) processBatch(
	ctx context.Context,
	tx pgx.Tx,
	batchSize int,
	booksCh chan<- entity.Book,
) (bool, error) {
	rows, err := tx.Query(ctx, fmt.Sprintf(`FETCH FORWARD %d FROM author_books_cursor`, batchSize))
	if err != nil {
		return false, errors.Wrap(err, "fetch from author_books cursor error")
	}
	defer rows.Close()

	var hasData bool

	for rows.Next() {
		hasData = true
		var book entity.Book
		if err = rows.Scan(&book.ID, &book.Name, &book.CreatedAt, &book.UpdatedAt, &book.AuthorIDs); err != nil {
			return false, errors.Wrap(err, "scan error")
		}

		select {
		case booksCh <- book:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	return hasData, rows.Err()
}

func (p *postgresRepository) insertBatch(ctx context.Context, tx pgx.Tx, book entity.Book) error {
	p.logger.Debug("inserting batch of author-book relations", zap.String("book_id", book.ID))

	const queryInsert = `
INSERT INTO author_book (author_id, book_id)
VALUES ($1, $2)
`
	batch := &pgx.Batch{}

	for _, authorID := range book.AuthorIDs {
		batch.Queue(queryInsert, authorID, book.ID)
	}

	res := tx.SendBatch(ctx, batch)

	for _, authorID := range book.AuthorIDs {
		_, err := res.Exec()
		if err != nil {
			return p.checkForeignKeyError(err, authorID)
		}
	}

	if err := res.Close(); err != nil {
		return errors.Wrap(err, "failed to close batch results (inserting author-book relations)")
	}
	p.logger.Debug("successfully inserted batch of author-book relations", zap.String("book_id", book.ID))
	return nil
}

func (p *postgresRepository) checkForeignKeyError(err error, authorID string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return fmt.Errorf("%w: %s not found", entity.ErrAuthorNotFound, authorID)
	}
	return err
}

func (p *postgresRepository) rollbackErrorHandle(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil {
		p.logger.Debug("failed to rollback transaction", zap.Error(err))
	}
}
