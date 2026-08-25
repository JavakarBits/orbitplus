package master

import (
	"context"
	"errors"

	"orbitplusmaster/internal/domain"
)

var (
	// ErrInvalidOperatorCode indicates that an operator code is blank or unsafe.
	ErrInvalidOperatorCode = errors.New("invalid operator code")
	// ErrInvalidOperatorZoneCode indicates that a zone code is not supported.
	ErrInvalidOperatorZoneCode = errors.New("invalid operator zone code")
	// ErrOperatorNotFound indicates that an operator does not exist in the registry.
	ErrOperatorNotFound = errors.New("operator not found")
	// ErrOperatorRegistryUnavailable indicates that registry storage cannot be reached.
	ErrOperatorRegistryUnavailable = errors.New("operator registry unavailable")
)

// OperatorRegistry manages discovered operators and lists refresh targets.
type OperatorRegistry interface {
	RegisterOperator(context.Context, string, string) (domain.Operator, error)
	ListActiveOperators(context.Context) ([]domain.Operator, error)
	ListOperators(context.Context) ([]domain.Operator, error)
	SetOperatorActive(context.Context, string, bool) (domain.Operator, error)
}
