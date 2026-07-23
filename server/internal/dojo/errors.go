package dojo

import (
	"errors"
	"fmt"
	"net/http"
)

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	Details    map[string]any
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func apiError(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

func asAPIError(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return &Error{
		Code:       "internal_error",
		Message:    "internal server error",
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}

var (
	ErrDeviceNotFound      = errors.New("device not found")
	ErrNonceNotFound       = errors.New("nonce not found")
	ErrNonceUsed           = errors.New("nonce already used")
	ErrReplayDetected      = errors.New("replay detected")
	ErrIdempotencyConflict = errors.New("idempotency key reused with another request")
	ErrSubmissionRate      = errors.New("submission interval is too short")
	ErrDailyLimit          = errors.New("daily submission limit reached")
)
