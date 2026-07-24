package device

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const (
	defaultChallengeTTL         = 5 * time.Minute
	maxIdentityLength           = 128
	registrationSignatureDomain = "gochya-device-enrollment-signature-v1"
	playIntegrityDomain         = "gochya-device-enrollment-play-integrity-v1"
)

type ServiceConfig struct {
	Store            Store
	Attestation      dojo.AttestationVerifier
	AllowedAppBuilds map[string]struct{}
	Now              func() time.Time
	Random           io.Reader
	ChallengeTTL     time.Duration
}

type Service struct {
	store            Store
	attestation      dojo.AttestationVerifier
	allowedAppBuilds map[string]struct{}
	now              func() time.Time
	random           io.Reader
	challengeTTL     time.Duration
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Attestation == nil {
		return nil, errors.New("store and attestation verifier are required")
	}
	if len(config.AllowedAppBuilds) == 0 {
		return nil, errors.New("app build allowlist is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	if config.ChallengeTTL == 0 {
		config.ChallengeTTL = defaultChallengeTTL
	}
	if config.ChallengeTTL <= 0 {
		return nil, errors.New("challenge TTL must be positive")
	}
	return &Service{
		store:            config.Store,
		attestation:      config.Attestation,
		allowedAppBuilds: cloneSet(config.AllowedAppBuilds),
		now:              config.Now,
		random:           config.Random,
		challengeTTL:     config.ChallengeTTL,
	}, nil
}

func (s *Service) Preflight(
	ctx context.Context,
	playerID string,
	request PreflightRequest,
) (PreflightResponse, error) {
	if err := s.validateEnvelope(playerID, request.DeviceID, request.Platform, request.AppBuild); err != nil {
		return PreflightResponse{}, err
	}
	challenge, err := randomToken(s.random, 32)
	if err != nil {
		return PreflightResponse{}, asAPIError(fmt.Errorf("generate enrollment challenge: %w", err))
	}
	now := s.now().UTC()
	record := ChallengeRecord{
		Value:     challenge,
		PlayerID:  playerID,
		DeviceID:  request.DeviceID,
		Platform:  request.Platform,
		AppBuild:  request.AppBuild,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.challengeTTL),
	}
	if err := s.store.PutChallenge(ctx, record); err != nil {
		return PreflightResponse{}, asAPIError(fmt.Errorf("store enrollment challenge: %w", err))
	}
	return PreflightResponse{
		Challenge: record.Value,
		ExpiresAt: record.ExpiresAt,
	}, nil
}

func (s *Service) Register(
	ctx context.Context,
	playerID string,
	request RegisterRequest,
) (RegisterResponse, error) {
	if err := s.validateEnvelope(playerID, request.DeviceID, request.Platform, request.AppBuild); err != nil {
		return RegisterResponse{}, err
	}
	challenge, err := s.store.Challenge(ctx, request.Challenge)
	if err != nil ||
		challenge.PlayerID != playerID ||
		challenge.DeviceID != request.DeviceID ||
		challenge.Platform != request.Platform ||
		challenge.AppBuild != request.AppBuild {
		return RegisterResponse{}, challengeInvalidError()
	}
	now := s.now().UTC()
	if !now.Before(challenge.ExpiresAt) {
		return RegisterResponse{}, challengeInvalidError()
	}
	if challenge.UsedAt != nil {
		return RegisterResponse{}, replayError()
	}

	publicKey, err := decodeCanonicalBase64URL(request.PublicKey, ed25519.PublicKeySize)
	if err != nil {
		return RegisterResponse{}, apiError(
			"public_key_invalid",
			"public key must be an unpadded base64url Ed25519 public key",
			http.StatusBadRequest,
		)
	}
	signature, err := decodeCanonicalBase64URL(request.ProofSignature, ed25519.SignatureSize)
	if err != nil {
		return RegisterResponse{}, signatureInvalidError()
	}
	canonical, err := CanonicalRegistrationPayload(request)
	if err != nil {
		return RegisterResponse{}, asAPIError(fmt.Errorf("canonical registration payload: %w", err))
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), canonical, signature) {
		return RegisterResponse{}, signatureInvalidError()
	}

	requestHash := PlayIntegrityRequestHash(request.Challenge, canonical)
	if err := s.attestation.Verify(ctx, dojo.AttestationInput{
		PlayerID:    playerID,
		DeviceID:    request.DeviceID,
		AppBuild:    request.AppBuild,
		Nonce:       request.Challenge,
		Challenge:   request.Challenge,
		RequestHash: requestHash,
		Evidence:    request.Attestation,
	}); err != nil {
		var unavailable *dojo.AttestationUnavailableError
		if errors.As(err, &unavailable) {
			return RegisterResponse{}, &Error{
				Code:       "attestation_unavailable",
				Message:    "device attestation is temporarily unavailable",
				HTTPStatus: http.StatusServiceUnavailable,
				Cause:      err,
			}
		}
		return RegisterResponse{}, &Error{
			Code:       "attestation_invalid",
			Message:    "device attestation is invalid",
			HTTPStatus: http.StatusUnauthorized,
			Cause:      err,
		}
	}

	registered, err := s.store.CommitRegistration(ctx, RegistrationCommit{
		Challenge: request.Challenge,
		PlayerID:  playerID,
		DeviceID:  request.DeviceID,
		PublicKey: ed25519.PublicKey(publicKey),
		Platform:  request.Platform,
		AppBuild:  request.AppBuild,
		Now:       now,
	})
	switch {
	case errors.Is(err, ErrChallengeUsed):
		return RegisterResponse{}, replayError()
	case errors.Is(err, ErrChallengeNotFound):
		return RegisterResponse{}, challengeInvalidError()
	case errors.Is(err, ErrDeviceConflict):
		return RegisterResponse{}, apiError(
			"device_key_conflict",
			"device is already registered with another key or state",
			http.StatusConflict,
		)
	case err != nil:
		return RegisterResponse{}, asAPIError(fmt.Errorf("commit device registration: %w", err))
	}
	return RegisterResponse{
		DeviceID:     registered.DeviceID,
		Platform:     registered.Platform,
		RegisteredAt: registered.RegisteredAt,
	}, nil
}

// CanonicalRegistrationPayload is the exact byte sequence signed by the new
// device key. The attestation request hash is derived from these same bytes.
func CanonicalRegistrationPayload(request RegisterRequest) ([]byte, error) {
	payload := struct {
		Version   uint8  `json:"version"`
		DeviceID  string `json:"deviceId"`
		Platform  string `json:"platform"`
		AppBuild  string `json:"appBuild"`
		Challenge string `json:"challenge"`
		PublicKey string `json:"publicKey"`
	}{
		Version:   1,
		DeviceID:  request.DeviceID,
		Platform:  request.Platform,
		AppBuild:  request.AppBuild,
		Challenge: request.Challenge,
		PublicKey: request.PublicKey,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	canonical := make([]byte, 0, len(registrationSignatureDomain)+1+len(encoded))
	canonical = append(canonical, registrationSignatureDomain...)
	canonical = append(canonical, 0)
	canonical = append(canonical, encoded...)
	return canonical, nil
}

func PlayIntegrityRequestHash(challenge string, canonical []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(playIntegrityDomain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(challenge))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(canonical)
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

func (s *Service) validateEnvelope(playerID, deviceID, platform, appBuild string) error {
	if strings.TrimSpace(playerID) == "" || strings.TrimSpace(deviceID) == "" ||
		len(playerID) > maxIdentityLength || len(deviceID) > maxIdentityLength {
		return apiError("identity_invalid", "player and device are required", http.StatusBadRequest)
	}
	if platform != PlatformWearOS {
		return apiError(
			"platform_unsupported",
			"device enrollment is not available for this platform",
			http.StatusBadRequest,
		)
	}
	if _, ok := s.allowedAppBuilds[appBuild]; !ok {
		return apiError("unsupported_build", "app build is not allowed", http.StatusBadRequest)
	}
	return nil
}

func decodeCanonicalBase64URL(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != size {
		return nil, errors.New("invalid base64url value")
	}
	if base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("non-canonical base64url value")
	}
	return decoded, nil
}

func randomToken(reader io.Reader, length int) (string, error) {
	value := make([]byte, length)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	destination := make(map[string]struct{}, len(source))
	for value := range source {
		destination[value] = struct{}{}
	}
	return destination
}

func challengeInvalidError() *Error {
	return apiError(
		"enrollment_challenge_invalid",
		"device enrollment challenge is unknown, expired or does not match",
		http.StatusConflict,
	)
}

func replayError() *Error {
	return apiError(
		"enrollment_replay_detected",
		"device enrollment challenge was already used",
		http.StatusConflict,
	)
}

func signatureInvalidError() *Error {
	return apiError(
		"signature_invalid",
		"device key proof signature is invalid",
		http.StatusUnauthorized,
	)
}
