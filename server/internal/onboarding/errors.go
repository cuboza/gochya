package onboarding

import (
	"errors"
	"net/http"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
)

var (
	ErrPlayerNotFound          = errors.New("player not found")
	ErrAgeGateLocked           = errors.New("age gate is locked")
	ErrAgeGateRequired         = errors.New("age gate is required")
	ErrParentalConsentRequired = errors.New("parental consent is required")
	ErrStarterAlreadySelected  = errors.New("starter egg was already selected")
	ErrStarterUnavailable      = errors.New("starter egg is unavailable")
	ErrIdempotencyConflict     = errors.New("idempotency conflict")
	ErrGenomeInvalid           = errors.New("starter genome is invalid")
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

func mapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrPlayerNotFound):
		return apiError("profile_not_found", "player profile was not found", http.StatusNotFound)
	case errors.Is(err, ErrAgeGateLocked):
		return apiError(
			"age_gate_locked",
			"the recorded age category cannot be changed",
			http.StatusConflict,
		)
	case errors.Is(err, ErrAgeGateRequired):
		return apiError(
			"age_gate_required",
			"complete the age gate before selecting a starter egg",
			http.StatusConflict,
		)
	case errors.Is(err, ErrParentalConsentRequired):
		return apiError(
			"parental_consent_required",
			"verified parental consent is required before continuing",
			http.StatusForbidden,
		)
	case errors.Is(err, ErrStarterAlreadySelected):
		return apiError(
			"starter_already_selected",
			"a different starter egg was already selected",
			http.StatusConflict,
		)
	case errors.Is(err, ErrStarterUnavailable):
		return apiError(
			"starter_unavailable",
			"starter selection is only available before a pet or egg exists",
			http.StatusConflict,
		)
	case errors.Is(err, ErrIdempotencyConflict):
		return apiError(
			"idempotency_conflict",
			"Idempotency-Key was already used with another onboarding request",
			http.StatusConflict,
		)
	case errors.Is(err, ErrGenomeInvalid):
		return apiError(
			"genome_invalid",
			"generated starter genome is invalid",
			http.StatusConflict,
		)
	case errors.Is(err, corebridge.ErrUnavailable):
		return apiError(
			"core_unavailable",
			"Gochya Core is unavailable",
			http.StatusServiceUnavailable,
		)
	default:
		return asAPIError(err)
	}
}
