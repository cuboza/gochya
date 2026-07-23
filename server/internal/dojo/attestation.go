package dojo

import (
	"context"
	"errors"
	"fmt"
)

type AttestationInput struct {
	PlayerID    string
	DeviceID    string
	AppBuild    string
	Nonce       string
	Challenge   string
	RequestHash string
	Evidence    AttestationEvidence
}

type AttestationVerifier interface {
	Verify(context.Context, AttestationInput) error
}

type AttestationVerifierFunc func(context.Context, AttestationInput) error

func (fn AttestationVerifierFunc) Verify(ctx context.Context, input AttestationInput) error {
	return fn(ctx, input)
}

// RejectingAttestationVerifier keeps an accidentally started API fail-closed until
// the Play Integrity verifier is configured.
type RejectingAttestationVerifier struct{}

func (RejectingAttestationVerifier) Verify(context.Context, AttestationInput) error {
	return errors.New("attestation verifier is not configured")
}

type AttestationUnavailableError struct {
	Cause error
}

func (e *AttestationUnavailableError) Error() string {
	return fmt.Sprintf("attestation service is unavailable: %v", e.Cause)
}

func (e *AttestationUnavailableError) Unwrap() error {
	return e.Cause
}
