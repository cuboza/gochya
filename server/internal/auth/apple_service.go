package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	appleLoginProvider        = "apple"
	appleLoginNonceBytes      = 32
	defaultAppleLoginNonceTTL = 5 * time.Minute
	maxAppleLoginNonceTTL     = 15 * time.Minute
)

type AppleExchangeServiceConfig struct {
	Verifier AppleIdentityVerifier
	Nonces   LoginNonceStore
	Players  IdentityStore
	Sessions SessionIssuer
	NonceTTL time.Duration
	Now      func() time.Time
	Random   io.Reader
}

type AppleExchangeService struct {
	verifier AppleIdentityVerifier
	nonces   LoginNonceStore
	players  IdentityStore
	sessions SessionIssuer
	nonceTTL time.Duration
	now      func() time.Time
	random   io.Reader
}

func NewAppleExchangeService(
	config AppleExchangeServiceConfig,
) (*AppleExchangeService, error) {
	if config.Verifier == nil ||
		config.Nonces == nil ||
		config.Players == nil ||
		config.Sessions == nil {
		return nil, errors.New(
			"Apple verifier, nonce store, identity store and session issuer are required",
		)
	}
	if config.NonceTTL == 0 {
		config.NonceTTL = defaultAppleLoginNonceTTL
	}
	if config.NonceTTL <= 0 || config.NonceTTL > maxAppleLoginNonceTTL {
		return nil, errors.New(
			"Apple login nonce TTL must be between zero and 15 minutes",
		)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &AppleExchangeService{
		verifier: config.Verifier,
		nonces:   config.Nonces,
		players:  config.Players,
		sessions: config.Sessions,
		nonceTTL: config.NonceTTL,
		now:      config.Now,
		random:   config.Random,
	}, nil
}

func (s *AppleExchangeService) Preflight(
	ctx context.Context,
) (ApplePreflightResponse, error) {
	nonce, err := randomAppleLoginNonce(s.random)
	if err != nil {
		return ApplePreflightResponse{}, fmt.Errorf(
			"generate Apple login nonce: %w",
			err,
		)
	}
	now := s.now().UTC()
	record := LoginNonceRecord{
		Provider:  appleLoginProvider,
		Nonce:     nonce,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.nonceTTL),
	}
	if err := s.nonces.Create(ctx, record); err != nil {
		return ApplePreflightResponse{}, fmt.Errorf(
			"store Apple login nonce: %w",
			err,
		)
	}
	return ApplePreflightResponse{
		Nonce:     record.Nonce,
		ExpiresAt: record.ExpiresAt,
	}, nil
}

func (s *AppleExchangeService) Exchange(
	ctx context.Context,
	identityToken string,
	nonce string,
	deviceID string,
) (LoginResponse, error) {
	if identityToken == "" ||
		nonce == "" ||
		len(deviceID) > maxAuthDeviceIDLength {
		return LoginResponse{}, ErrLoginRequestInvalid
	}
	if !validAppleLoginNonce(nonce) {
		return LoginResponse{}, ErrLoginNonceInvalid
	}
	identity, err := s.verifier.Verify(ctx, identityToken, nonce)
	if err != nil {
		return LoginResponse{}, err
	}
	if identity.Provider != appleLoginProvider ||
		strings.TrimSpace(identity.Subject) == "" {
		return LoginResponse{}, ErrIdentityTokenInvalid
	}
	now := s.now().UTC()
	if err := s.nonces.Consume(
		ctx,
		appleLoginProvider,
		nonce,
		"",
		now,
	); err != nil {
		return LoginResponse{}, err
	}
	playerID, err := randomUUID(s.random)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("generate player ID: %w", err)
	}
	player, err := s.players.Resolve(ctx, PlayerCandidate{
		ID:          playerID,
		Username:    providerUsername(identity.Provider, identity.Subject),
		DisplayName: identity.DisplayName,
		Identity:    identity,
		Now:         now,
	})
	if err != nil {
		return LoginResponse{}, fmt.Errorf("resolve Apple player: %w", err)
	}
	pair, err := s.sessions.Issue(ctx, player.ID, strings.TrimSpace(deviceID))
	if err != nil {
		return LoginResponse{}, fmt.Errorf("issue Apple session: %w", err)
	}
	return LoginResponse{TokenPair: pair, Player: player}, nil
}

func randomAppleLoginNonce(reader io.Reader) (string, error) {
	value := make([]byte, appleLoginNonceBytes)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validAppleLoginNonce(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil &&
		len(decoded) == appleLoginNonceBytes &&
		base64.RawURLEncoding.EncodeToString(decoded) == value
}

func loginNonceDigest(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}
