package shop

import (
	"errors"
	"net/http"

	"github.com/gochya/gochya/server/internal/dojo"
)

var (
	ErrPlayerNotFound      = errors.New("player not found")
	ErrInsufficientKoins   = errors.New("insufficient Koins")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrInventoryOverflow   = errors.New("inventory quantity overflow")
	ErrStoredResponse      = errors.New("stored shop response is invalid")
)

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func apiError(code string, message string, status int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

func asAPIError(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	var dojoError *dojo.Error
	if errors.As(err, &dojoError) {
		return &Error{
			Code:       dojoError.Code,
			Message:    dojoError.Message,
			HTTPStatus: dojoError.HTTPStatus,
			Cause:      dojoError,
		}
	}
	switch {
	case errors.Is(err, ErrPlayerNotFound):
		return apiError("profile_not_found", "player profile was not found", http.StatusNotFound)
	case errors.Is(err, ErrInsufficientKoins):
		return apiError(
			"insufficient_koins",
			"the player does not have enough Koins",
			http.StatusConflict,
		)
	case errors.Is(err, ErrIdempotencyConflict):
		return apiError(
			"idempotency_conflict",
			"Idempotency-Key was already used with another purchase",
			http.StatusConflict,
		)
	case errors.Is(err, ErrInventoryOverflow):
		return apiError(
			"inventory_limit",
			"the item quantity cannot be increased",
			http.StatusConflict,
		)
	default:
		return &Error{
			Code:       "internal_error",
			Message:    "internal server error",
			HTTPStatus: http.StatusInternalServerError,
			Cause:      err,
		}
	}
}
