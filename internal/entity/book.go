package entity

import (
	"time"

	"github.com/pkg/errors"
)

type Book struct {
	ID        string
	Name      string
	AuthorIDs []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrBookNotFound      = errors.New("book not found")
	ErrBookAlreadyExists = errors.New("book already exists")
)
