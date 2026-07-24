package device

import (
	"crypto/ed25519"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const PlatformWearOS = "wear_os"

type PreflightRequest struct {
	DeviceID string `json:"deviceId"`
	Platform string `json:"platform"`
	AppBuild string `json:"appBuild"`
}

type PreflightResponse struct {
	Challenge string    `json:"challenge"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type RegisterRequest struct {
	DeviceID       string                   `json:"deviceId"`
	Platform       string                   `json:"platform"`
	AppBuild       string                   `json:"appBuild"`
	Challenge      string                   `json:"challenge"`
	PublicKey      string                   `json:"publicKey"`
	Attestation    dojo.AttestationEvidence `json:"attestation"`
	ProofSignature string                   `json:"proofSignature"`
}

type RegisterResponse struct {
	DeviceID     string    `json:"deviceId"`
	Platform     string    `json:"platform"`
	RegisteredAt time.Time `json:"registeredAt"`
}

type ChallengeRecord struct {
	Value     string
	PlayerID  string
	DeviceID  string
	Platform  string
	AppBuild  string
	IssuedAt  time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type RegisteredDevice struct {
	PlayerID     string
	DeviceID     string
	PublicKey    ed25519.PublicKey
	Platform     string
	Enabled      bool
	RegisteredAt time.Time
}

type RegistrationCommit struct {
	Challenge string
	PlayerID  string
	DeviceID  string
	PublicKey ed25519.PublicKey
	Platform  string
	AppBuild  string
	Now       time.Time
}
