package activity

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gochya/gochya/server/internal/dojo"
)

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

func apiError(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

func asAPIError(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	var dojoErr *dojo.Error
	if errors.As(err, &dojoErr) {
		return &Error{
			Code:       dojoErr.Code,
			Message:    dojoErr.Message,
			HTTPStatus: dojoErr.HTTPStatus,
			Cause:      dojoErr,
		}
	}
	return &Error{
		Code:       "internal_error",
		Message:    "internal server error",
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}
