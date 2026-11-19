package domain

import "errors"

// Erros de negócio. O service retorna (ou embrulha com %w) estes valores
// e o handler usa errors.Is para decidir o status HTTP.
var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthenticated = errors.New("unauthenticated")

	ErrInventoryPrivate    = errors.New("steam inventory is private")
	ErrUpstreamRateLimited = errors.New("steam is rate limiting requests")
)
