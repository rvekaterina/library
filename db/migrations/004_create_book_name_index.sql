-- +goose Up
CREATE INDEX index_book_name ON book (name);

-- +goose Down
DROP INDEX index_book_name;