-- +goose Up
CREATE TABLE author_book
(
    author_id UUID REFERENCES author (id) ON DELETE CASCADE,
    book_id   UUID REFERENCES book (id) ON DELETE CASCADE,
    PRIMARY KEY (author_id, book_id)
);

-- +goose Down
DROP TABLE author_book;