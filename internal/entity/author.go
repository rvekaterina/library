package entity

import (
	"time"

	"github.com/pkg/errors"
)

type Author struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrAuthorNotFound      = errors.New("author not found")
	ErrAuthorAlreadyExists = errors.New("author already exists")
)
