package auth

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeAppleIdentityVerifier struct {
	identity ExternalIdentity
	err      error
	token    string
	nonce    string
}

func (verifier *fakeAppleIdentityVerifier) Verify(
	_ context.Context,
	token string,
	nonce string,
) (ExternalIdentity, error) {
	verifier.token = token
	verifier.nonce = nonce
	return verifier.identity, verifier.err
}

type fakeLoginNonceStore struct {
	mutex      sync.Mutex
	record     LoginNonceRecord
	used       bool
	createErr  error
	consumeErr error
}

func (store *fakeLoginNonceStore) Create(
	_ context.Context,
	record LoginNonceRecord,
) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.record = record
	return store.createErr
}

func (store *fakeLoginNonceStore) Consume(
	_ context.Context,
	provider string,
	nonce string,
	binding string,
	now time.Time,
) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.consumeErr != nil {
		return store.consumeErr
	}
	if store.used ||
		provider != store.record.Provider ||
		nonce != store.record.Nonce ||
		binding != store.record.Binding ||
		now.Before(store.record.IssuedAt) ||
		!now.Before(store.record.ExpiresAt) {
		return ErrLoginNonceInvalid
	}
	store.used = true
	return nil
}

func TestAppleExchangeUsesServerNonceAndIssuesSession(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	random := append(
		bytes.Repeat([]byte{3}, appleLoginNonceBytes),
		bytes.Repeat([]byte{4}, 16)...,
	)
	verifier := &fakeAppleIdentityVerifier{identity: ExternalIdentity{
		Provider: "apple",
		Subject:  "apple-account-123",
	}}
	nonces := &fakeLoginNonceStore{}
	players := &fakeIdentityStore{player: Player{
		ID:       "77777777-7777-4777-8777-777777777777",
		Username: "apple_stable",
	}}
	sessions := &fakeSessionIssuer{pair: TokenPair{JWT: "access"}}
	service, err := NewAppleExchangeService(AppleExchangeServiceConfig{
		Verifier: verifier,
		Nonces:   nonces,
		Players:  players,
		Sessions: sessions,
		Now:      func() time.Time { return now },
		Random:   bytes.NewReader(random),
	})
	if err != nil {
		t.Fatalf("NewAppleExchangeService: %v", err)
	}
	preflight, err := service.Preflight(context.Background())
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if !validAppleLoginNonce(preflight.Nonce) ||
		preflight.ExpiresAt != now.Add(defaultAppleLoginNonceTTL) ||
		nonces.record.Nonce != preflight.Nonce {
		t.Fatalf("preflight = %#v, record = %#v", preflight, nonces.record)
	}
	response, err := service.Exchange(
		context.Background(),
		"apple-identity-token",
		preflight.Nonce,
		" phone-1 ",
	)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if verifier.token != "apple-identity-token" ||
		verifier.nonce != preflight.Nonce ||
		!nonces.used ||
		players.candidate.Username != "apple_6ff05aab3b129831a943df7a" ||
		sessions.deviceID != "phone-1" ||
		response.JWT != "access" {
		t.Fatalf(
			"exchange state: verifier=%#v nonce=%#v candidate=%#v session=%#v response=%#v",
			verifier,
			nonces,
			players.candidate,
			sessions,
			response,
		)
	}
}

func TestAppleExchangeRejectsNonceReplayBeforePlayerResolution(t *testing.T) {
	nonce := testAppleNonce(12)
	players := &fakeIdentityStore{}
	service, err := NewAppleExchangeService(AppleExchangeServiceConfig{
		Verifier: &fakeAppleIdentityVerifier{identity: ExternalIdentity{
			Provider: "apple",
			Subject:  "apple-account-123",
		}},
		Nonces: &fakeLoginNonceStore{
			record: LoginNonceRecord{
				Provider:  appleLoginProvider,
				Nonce:     nonce,
				IssuedAt:  time.Now().Add(-time.Minute),
				ExpiresAt: time.Now().Add(time.Minute),
			},
			consumeErr: ErrLoginNonceInvalid,
		},
		Players:  players,
		Sessions: &fakeSessionIssuer{},
	})
	if err != nil {
		t.Fatalf("NewAppleExchangeService: %v", err)
	}
	_, err = service.Exchange(context.Background(), "token", nonce, "")
	if !errors.Is(err, ErrLoginNonceInvalid) ||
		players.candidate.Identity.Provider != "" {
		t.Fatalf("error = %v, candidate = %#v", err, players.candidate)
	}
}

func TestAppleExchangeDoesNotConsumeNonceForInvalidToken(t *testing.T) {
	nonces := &fakeLoginNonceStore{}
	service, err := NewAppleExchangeService(AppleExchangeServiceConfig{
		Verifier: &fakeAppleIdentityVerifier{err: ErrIdentityTokenInvalid},
		Nonces:   nonces,
		Players:  &fakeIdentityStore{},
		Sessions: &fakeSessionIssuer{},
	})
	if err != nil {
		t.Fatalf("NewAppleExchangeService: %v", err)
	}
	_, err = service.Exchange(
		context.Background(),
		"invalid-token",
		testAppleNonce(13),
		"",
	)
	if !errors.Is(err, ErrIdentityTokenInvalid) || nonces.used {
		t.Fatalf("error = %v, nonce used = %v", err, nonces.used)
	}
}
