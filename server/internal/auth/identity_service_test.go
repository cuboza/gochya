package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeGoogleIdentityVerifier struct {
	identity ExternalIdentity
	err      error
	token    string
}

func (verifier *fakeGoogleIdentityVerifier) Verify(
	_ context.Context,
	token string,
) (ExternalIdentity, error) {
	verifier.token = token
	return verifier.identity, verifier.err
}

type fakeIdentityStore struct {
	player    Player
	err       error
	candidate PlayerCandidate
}

func (store *fakeIdentityStore) Resolve(
	_ context.Context,
	candidate PlayerCandidate,
) (Player, error) {
	store.candidate = candidate
	return store.player, store.err
}

type fakeSessionIssuer struct {
	pair     TokenPair
	err      error
	playerID string
	deviceID string
}

func (issuer *fakeSessionIssuer) Issue(
	_ context.Context,
	playerID string,
	deviceID string,
) (TokenPair, error) {
	issuer.playerID = playerID
	issuer.deviceID = deviceID
	return issuer.pair, issuer.err
}

func TestGoogleExchangeResolvesPlayerAndIssuesSession(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	verifier := &fakeGoogleIdentityVerifier{identity: ExternalIdentity{
		Provider:    "google",
		Subject:     "google-account-123",
		DisplayName: "Gochya Player",
	}}
	players := &fakeIdentityStore{player: Player{
		ID:          "77777777-7777-4777-8777-777777777777",
		Username:    "google_123",
		DisplayName: "Gochya Player",
	}}
	sessions := &fakeSessionIssuer{pair: TokenPair{JWT: "access"}}
	service, err := NewGoogleExchangeService(GoogleExchangeServiceConfig{
		Verifier: verifier,
		Players:  players,
		Sessions: sessions,
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGoogleExchangeService: %v", err)
	}

	response, err := service.Exchange(
		context.Background(),
		"google-id-token",
		" phone-1 ",
	)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if verifier.token != "google-id-token" ||
		players.candidate.Identity.Subject != "google-account-123" ||
		players.candidate.Username != "google_7f62525643255d66bc92e360" {
		t.Fatalf("candidate = %#v", players.candidate)
	}
	if sessions.playerID != players.player.ID || sessions.deviceID != "phone-1" {
		t.Fatalf("session identity = %q, %q", sessions.playerID, sessions.deviceID)
	}
	if response.JWT != "access" || response.Player != players.player {
		t.Fatalf("response = %#v", response)
	}
}

func TestGoogleExchangeStopsBeforeSessionOnIdentityFailure(t *testing.T) {
	providerFailure := &IdentityProviderUnavailableError{
		Cause: errors.New("Google unavailable"),
	}
	service, err := NewGoogleExchangeService(GoogleExchangeServiceConfig{
		Verifier: &fakeGoogleIdentityVerifier{err: providerFailure},
		Players:  &fakeIdentityStore{},
		Sessions: &fakeSessionIssuer{},
	})
	if err != nil {
		t.Fatalf("NewGoogleExchangeService: %v", err)
	}
	_, err = service.Exchange(context.Background(), "token", "")
	var unavailable *IdentityProviderUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestGoogleExchangeRejectsOversizedDeviceID(t *testing.T) {
	verifier := &fakeGoogleIdentityVerifier{}
	service, err := NewGoogleExchangeService(GoogleExchangeServiceConfig{
		Verifier: verifier,
		Players:  &fakeIdentityStore{},
		Sessions: &fakeSessionIssuer{},
	})
	if err != nil {
		t.Fatalf("NewGoogleExchangeService: %v", err)
	}
	_, err = service.Exchange(
		context.Background(),
		"token",
		string(make([]byte, 129)),
	)
	if !errors.Is(err, ErrLoginRequestInvalid) || verifier.token != "" {
		t.Fatalf("error = %v, verifier token = %q", err, verifier.token)
	}
}
