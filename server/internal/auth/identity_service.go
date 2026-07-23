package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type GoogleExchangeServiceConfig struct {
	Verifier GoogleIdentityVerifier
	Players  IdentityStore
	Sessions SessionIssuer
	Now      func() time.Time
	Random   io.Reader
}

type GoogleExchangeService struct {
	verifier GoogleIdentityVerifier
	players  IdentityStore
	sessions SessionIssuer
	now      func() time.Time
	random   io.Reader
}

func NewGoogleExchangeService(
	config GoogleExchangeServiceConfig,
) (*GoogleExchangeService, error) {
	if config.Verifier == nil || config.Players == nil || config.Sessions == nil {
		return nil, errors.New(
			"Google verifier, identity store and session issuer are required",
		)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &GoogleExchangeService{
		verifier: config.Verifier,
		players:  config.Players,
		sessions: config.Sessions,
		now:      config.Now,
		random:   config.Random,
	}, nil
}

func (s *GoogleExchangeService) Exchange(
	ctx context.Context,
	idToken string,
	deviceID string,
) (LoginResponse, error) {
	if len(deviceID) > 128 {
		return LoginResponse{}, ErrLoginRequestInvalid
	}
	identity, err := s.verifier.Verify(ctx, idToken)
	if err != nil {
		return LoginResponse{}, err
	}
	if identity.Provider != "google" ||
		strings.TrimSpace(identity.Subject) == "" {
		return LoginResponse{}, ErrIdentityTokenInvalid
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
		Now:         s.now().UTC(),
	})
	if err != nil {
		return LoginResponse{}, fmt.Errorf("resolve Google player: %w", err)
	}
	pair, err := s.sessions.Issue(ctx, player.ID, strings.TrimSpace(deviceID))
	if err != nil {
		return LoginResponse{}, fmt.Errorf("issue Google session: %w", err)
	}
	return LoginResponse{TokenPair: pair, Player: player}, nil
}

func providerUsername(provider string, subject string) string {
	digest := sha256.Sum256([]byte(provider + "\x00" + subject))
	return provider + "_" + hex.EncodeToString(digest[:12])
}
