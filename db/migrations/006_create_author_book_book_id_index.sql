-- +goose Up
CREATE INDEX index_author_book_book_id ON author_book (book_id);

-- +goose Down
DROP INDEX index_author_book_book_id;