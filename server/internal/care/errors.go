package care

import (
	"errors"
	"net/http"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
)

var (
	ErrPlayerNotFound = errors.New("player not found")
	ErrPetNotFound    = errors.New("pet not found")
	ErrPetState       = errors.New("pet care state is invalid")
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
	case errors.Is(err, ErrPetNotFound):
		return apiError("pet_not_found", "pet was not found", http.StatusNotFound)
	case errors.Is(err, ErrPetState):
		return apiError(
			"pet_state_invalid",
			"stored pet care state is invalid",
			http.StatusConflict,
		)
	case errors.Is(err, corebridge.ErrUnavailable):
		return apiError(
			"core_unavailable",
			"Gochya Core is unavailable",
			http.StatusServiceUnavailable,
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
