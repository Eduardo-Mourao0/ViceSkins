package domain

import "errors"

// Erros de negócio. O service retorna (ou embrulha com %w) estes valores
// e o handler usa errors.Is para decidir o status HTTP.
var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
	ErrInvalidInput    = errors.New("invalid input")

	ErrInventoryPrivate    = errors.New("steam inventory is private")
	ErrUpstreamRateLimited = errors.New("steam is rate limiting requests")

	ErrItemNotTradable   = errors.New("item is not tradable")
	ErrItemAlreadyListed = errors.New("item already has an active listing")
	ErrListingNotActive  = errors.New("listing is not active")

	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrCannotBuyOwnListing = errors.New("cannot buy your own listing")
	ErrInvalidTransition   = errors.New("invalid transaction status transition")
)
