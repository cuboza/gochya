package dojo

import (
	"context"
	"time"
)

type CommitRequest struct {
	PlayerID              string
	DeviceID              string
	AppBuild              string
	ClassifierVersion     string
	EvidenceSchemaVersion uint16
	Nonce                 string
	IdempotencyKey        string
	RequestHash           [32]byte
	ReplayHash            [32]byte
	Response              SubmitResponse
	Now                   time.Time
}

type Store interface {
	Device(context.Context, string, string) (Device, error)
	ActiveElement(context.Context, string) (uint8, error)
	PutNonce(context.Context, NonceRecord) error
	Nonce(context.Context, string) (NonceRecord, error)
	Idempotency(context.Context, string, string) (SubmitResponse, [32]byte, bool, error)
	CommitSubmit(context.Context, CommitRequest) (SubmitResponse, error)
}
