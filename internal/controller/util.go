package controller

import (
	"github.com/pkg/errors"
	"github.com/rvekaterina/library/internal/entity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (i *implementation) convertErr(err error) error {
	switch {
	case errors.Is(err, entity.ErrAuthorNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, entity.ErrBookNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, entity.ErrAuthorAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, entity.ErrBookAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
