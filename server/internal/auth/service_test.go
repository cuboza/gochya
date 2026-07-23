package auth_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/auth"
	"github.com/gochya/gochya/server/internal/dojo"
)

const testPlayerID = "77777777-7777-4777-8777-777777777777"

type recordingRefreshStore struct {
	created     auth.RefreshTokenRecord
	createErr   error
	rotateHash  [32]byte
	replacement auth.RefreshTokenReplacement
	identity    auth.RefreshIdentity
	rotateErr   error
	revokeHash  [32]byte
	revokeErr   error
}

func (store *recordingRefreshStore) Create(
	_ context.Context,
	record auth.RefreshTokenRecord,
) error {
	store.created = record
	return store.createErr
}

func (store *recordingRefreshStore) Rotate(
	_ context.Context,
	hash [32]byte,
	replacement auth.RefreshTokenReplacement,
	_ time.Time,
) (auth.RefreshIdentity, error) {
	store.rotateHash = hash
	store.replacement = replacement
	return store.identity, store.rotateErr
}

func (store *recordingRefreshStore) RevokeFamily(
	_ context.Context,
	hash [32]byte,
	_ time.Time,
) error {
	store.revokeHash = hash
	return store.revokeErr
}

func TestServiceIssuesVerifierCompatibleSession(t *testing.T) {
	store := &recordingRefreshStore{}
	service, publicKey, now := newTestService(t, store)

	pair, err := service.Issue(context.Background(), testPlayerID, "phone-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	assertAccessToken(t, pair.JWT, publicKey, now)
	if pair.AccessTokenExpiresAt != now.Add(15*time.Minute) ||
		pair.RefreshTokenExpiresAt != now.Add(30*24*time.Hour) {
		t.Fatalf("token expirations = %v, %v", pair.AccessTokenExpiresAt, pair.RefreshTokenExpiresAt)
	}
	if store.created.PlayerID != testPlayerID ||
		store.created.DeviceID != "phone-1" ||
		store.created.ID != store.created.FamilyID {
		t.Fatalf("stored refresh record = %#v", store.created)
	}
	if store.created.TokenHash != sha256.Sum256([]byte(pair.RefreshToken)) {
		t.Fatal("store did not receive the refresh-token hash")
	}
	if bytes.Contains(store.created.TokenHash[:], []byte(pair.RefreshToken)) {
		t.Fatal("stored hash contains plaintext refresh token")
	}
}

func TestServiceRotatesRefreshToken(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	store := &recordingRefreshStore{
		identity: auth.RefreshIdentity{
			PlayerID:        testPlayerID,
			DeviceID:        "phone-1",
			FamilyID:        "88888888-8888-4888-8888-888888888888",
			ExpiresAt:       now.Add(30 * 24 * time.Hour),
			FamilyExpiresAt: now.Add(90 * 24 * time.Hour),
		},
	}
	service, publicKey, _ := newTestService(t, store)
	current := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))

	pair, err := service.Refresh(context.Background(), current)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if pair.RefreshToken == current {
		t.Fatal("refresh token was not rotated")
	}
	if store.rotateHash != sha256.Sum256([]byte(current)) ||
		store.replacement.TokenHash != sha256.Sum256([]byte(pair.RefreshToken)) {
		t.Fatal("rotation hashes do not match their tokens")
	}
	assertAccessToken(t, pair.JWT, publicKey, now)
}

func TestServiceRejectsMalformedRefreshWithoutStoreLookup(t *testing.T) {
	store := &recordingRefreshStore{}
	service, _, _ := newTestService(t, store)
	if _, err := service.Refresh(context.Background(), "not-a-token"); !errors.Is(
		err,
		auth.ErrRefreshTokenInvalid,
	) {
		t.Fatalf("Refresh error = %v", err)
	}
	if store.rotateHash != ([32]byte{}) {
		t.Fatal("malformed token reached the store")
	}
}

func TestLogoutIsEnumerationSafe(t *testing.T) {
	store := &recordingRefreshStore{}
	service, _, _ := newTestService(t, store)
	if err := service.Logout(context.Background(), "not-a-token"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if store.revokeHash != ([32]byte{}) {
		t.Fatal("malformed logout token reached the store")
	}
}

func newTestService(
	t *testing.T,
	store auth.RefreshTokenStore,
) (*auth.Service, ed25519.PublicKey, time.Time) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := bytes.Clone(privateKey.Public().(ed25519.PublicKey))
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	service, err := auth.NewService(auth.ServiceConfig{
		Store:      store,
		KeyID:      "primary",
		PrivateKey: privateKey,
		Issuer:     "https://auth.gochya.test",
		Audience:   "gochya-api",
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	for index := range privateKey {
		privateKey[index] ^= 0xff
	}
	return service, publicKey, now
}

func assertAccessToken(
	t *testing.T,
	serialized string,
	publicKey ed25519.PublicKey,
	now time.Time,
) {
	t.Helper()
	verifier, err := dojo.NewJWTAuthenticator(dojo.JWTAuthenticatorConfig{
		Issuer:     "https://auth.gochya.test",
		Audience:   "gochya-api",
		PublicKeys: map[string]ed25519.PublicKey{"primary": publicKey},
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+serialized)
	playerID, err := verifier.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate issued access token: %v", err)
	}
	if playerID != testPlayerID {
		t.Fatalf("access-token subject = %q", playerID)
	}
}
