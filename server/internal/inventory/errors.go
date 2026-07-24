package inventory

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
	var dojoError *dojo.Error
	if errors.As(err, &dojoError) {
		return &Error{
			Code:       dojoError.Code,
			Message:    dojoError.Message,
			HTTPStatus: dojoError.HTTPStatus,
			Details:    dojoError.Details,
			Cause:      dojoError.Cause,
		}
	}
	return &Error{
		Code:       "internal_error",
		Message:    "internal server error",
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}
