package domain

import "errors"

// Erros de negócio. O service retorna (ou embrulha com %w) estes valores
// e o handler usa errors.Is para decidir o status HTTP.
var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthenticated = errors.New("unauthenticated")
)
