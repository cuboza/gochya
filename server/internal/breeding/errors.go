package breeding

import (
	"errors"
	"net/http"

	"github.com/gochya/gochya/server/internal/dojo"
)

var (
	ErrPlayerNotFound      = errors.New("player not found")
	ErrParentNotFound      = errors.New("breeding parent not found")
	ErrParentStateInvalid  = errors.New("breeding parent state invalid")
	ErrParentsIdentical    = errors.New("breeding parents are identical")
	ErrParentsTooRelated   = errors.New("breeding parents are too related")
	ErrParentCooldown      = errors.New("breeding parent is on cooldown")
	ErrInsufficientKoins   = errors.New("insufficient koins")
	ErrLoveCrystalRequired = errors.New("love crystal is required")
	ErrCatalystRequired    = errors.New("requested catalyst is required")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrEggNotFound         = errors.New("egg not found")
	ErrEggNotReady         = errors.New("egg is not ready")
	ErrGenomeInvalid       = errors.New("genome is invalid")
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
	return &Error{
		Code:       "internal_error",
		Message:    "internal server error",
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}
