package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeAppleSigningKeyProvider struct {
	key *rsa.PublicKey
	err error
	kid string
}

func (provider *fakeAppleSigningKeyProvider) SigningKey(
	_ context.Context,
	keyID string,
) (*rsa.PublicKey, error) {
	provider.kid = keyID
	return provider.key, provider.err
}

func TestAppleVerifierAcceptsValidIdentity(t *testing.T) {
	privateKey := testRSAKey(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	nonce := testAppleNonce(7)
	provider := &fakeAppleSigningKeyProvider{key: &privateKey.PublicKey}
	verifier := newTestAppleVerifier(t, provider, now)

	identity, err := verifier.Verify(
		context.Background(),
		signedAppleToken(t, privateKey, "apple-test-key", validAppleClaims(now, nonce)),
		nonce,
	)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if provider.kid != "apple-test-key" ||
		identity.Provider != "apple" ||
		identity.Subject != "apple-account-123" ||
		identity.DisplayName != "" {
		t.Fatalf("identity = %#v, key ID = %q", identity, provider.kid)
	}
}

func TestAppleVerifierRejectsInvalidClaims(t *testing.T) {
	privateKey := testRSAKey(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	nonce := testAppleNonce(8)
	tests := []struct {
		name   string
		mutate func(jwt.MapClaims)
	}{
		{
			name: "wrong issuer",
			mutate: func(claims jwt.MapClaims) {
				claims["iss"] = "https://attacker.example"
			},
		},
		{
			name: "wrong audience",
			mutate: func(claims jwt.MapClaims) {
				claims["aud"] = "com.attacker.app"
			},
		},
		{
			name: "wrong nonce",
			mutate: func(claims jwt.MapClaims) {
				claims["nonce"] = testAppleNonce(9)
			},
		},
		{
			name: "missing subject",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "sub")
			},
		},
		{
			name: "expired",
			mutate: func(claims jwt.MapClaims) {
				claims["exp"] = now.Unix()
			},
		},
		{
			name: "missing issued at",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "iat")
			},
		},
		{
			name: "future issued at",
			mutate: func(claims jwt.MapClaims) {
				claims["iat"] = now.Add(3 * time.Minute).Unix()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := validAppleClaims(now, nonce)
			test.mutate(claims)
			verifier := newTestAppleVerifier(
				t,
				&fakeAppleSigningKeyProvider{key: &privateKey.PublicKey},
				now,
			)
			_, err := verifier.Verify(
				context.Background(),
				signedAppleToken(t, privateKey, "apple-test-key", claims),
				nonce,
			)
			if !errors.Is(err, ErrIdentityTokenInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAppleVerifierRejectsUnexpectedAlgorithmAndUnknownKey(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	nonce := testAppleNonce(10)
	_, edPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, validAppleClaims(now, nonce))
	token.Header["kid"] = "unexpected-key"
	serialized, err := token.SignedString(edPrivate)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	verifier := newTestAppleVerifier(
		t,
		&fakeAppleSigningKeyProvider{err: errAppleSigningKeyNotFound},
		now,
	)
	if _, err := verifier.Verify(
		context.Background(),
		serialized,
		nonce,
	); !errors.Is(err, ErrIdentityTokenInvalid) {
		t.Fatalf("unexpected-algorithm error = %v", err)
	}

	rsaKey := testRSAKey(t)
	serialized = signedAppleToken(
		t,
		rsaKey,
		"unknown-key",
		validAppleClaims(now, nonce),
	)
	if _, err := verifier.Verify(
		context.Background(),
		serialized,
		nonce,
	); !errors.Is(err, ErrIdentityTokenInvalid) {
		t.Fatalf("unknown-key error = %v", err)
	}
}

func TestAppleVerifierClassifiesKeyFetchFailure(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	nonce := testAppleNonce(11)
	privateKey := testRSAKey(t)
	verifier := newTestAppleVerifier(
		t,
		&fakeAppleSigningKeyProvider{
			err: &appleSigningKeyFetchError{
				cause: &net.DNSError{IsTimeout: true, Err: "timeout"},
			},
		},
		now,
	)
	_, err := verifier.Verify(
		context.Background(),
		signedAppleToken(t, privateKey, "apple-test-key", validAppleClaims(now, nonce)),
		nonce,
	)
	var unavailable *IdentityProviderUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestAppleHTTPKeySetCachesAndRefreshesRotatedKey(t *testing.T) {
	firstKey := testRSAKey(t)
	secondKey := testRSAKey(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	currentKeyID := "first-key"
	currentKey := &firstKey.PublicKey
	requests := 0
	var mutex sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		mutex.Lock()
		defer mutex.Unlock()
		requests++
		if request.Method != http.MethodGet ||
			request.URL.String() != appleJWKURL ||
			request.Header.Get("Accept") != "application/json" {
			t.Fatalf("JWK request = %s %s %#v", request.Method, request.URL, request.Header)
		}
		document := appleJWKDocument(t, currentKeyID, currentKey)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Cache-Control": []string{"public, max-age=3600"},
			},
			Body:    io.NopCloser(strings.NewReader(document)),
			Request: request,
		}, nil
	})}
	keySet, err := NewAppleHTTPKeySet(AppleHTTPKeySetConfig{
		HTTPClient:         client,
		Now:                func() time.Time { return now },
		DefaultCacheTTL:    time.Hour,
		MinRefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewAppleHTTPKeySet: %v", err)
	}
	cached, err := keySet.SigningKey(context.Background(), "first-key")
	if err != nil {
		t.Fatalf("first SigningKey: %v", err)
	}
	cached.N.SetInt64(3)
	if _, err := keySet.SigningKey(
		context.Background(),
		"first-key",
	); err != nil {
		t.Fatalf("cached SigningKey: %v", err)
	}
	if requests != 1 {
		t.Fatalf("JWK requests after cache hit = %d", requests)
	}

	now = now.Add(2 * time.Second)
	mutex.Lock()
	currentKeyID = "second-key"
	currentKey = &secondKey.PublicKey
	mutex.Unlock()
	rotated, err := keySet.SigningKey(context.Background(), "second-key")
	if err != nil {
		t.Fatalf("rotated SigningKey: %v", err)
	}
	if rotated.N.Cmp(secondKey.PublicKey.N) != 0 || requests != 2 {
		t.Fatalf("rotated key mismatch or requests = %d", requests)
	}
}

func TestAppleHTTPKeySetThrottlesUnknownKeyRefresh(t *testing.T) {
	key := testRSAKey(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				appleJWKDocument(t, "known-key", &key.PublicKey),
			)),
			Request: request,
		}, nil
	})}
	keySet, err := NewAppleHTTPKeySet(AppleHTTPKeySetConfig{
		HTTPClient:      client,
		Now:             func() time.Time { return now },
		DefaultCacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewAppleHTTPKeySet: %v", err)
	}
	if _, err := keySet.SigningKey(
		context.Background(),
		"missing-a",
	); !errors.Is(err, errAppleSigningKeyNotFound) {
		t.Fatalf("first missing-key error = %v", err)
	}
	if _, err := keySet.SigningKey(
		context.Background(),
		"missing-b",
	); !errors.Is(err, errAppleSigningKeyNotFound) {
		t.Fatalf("second missing-key error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("unknown keys caused %d JWK requests", requests)
	}
}

func TestDecodeAppleJWKSetRejectsWeakAndDuplicateKeys(t *testing.T) {
	weakModulus := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 64))
	exponent := base64.RawURLEncoding.EncodeToString(big.NewInt(65537).Bytes())
	weak, err := json.Marshal(appleJWKSet{Keys: []appleJWK{{
		KeyType:   "RSA",
		KeyID:     "weak",
		Use:       "sig",
		Algorithm: "RS256",
		Modulus:   weakModulus,
		Exponent:  exponent,
	}}})
	if err != nil {
		t.Fatalf("Marshal weak key: %v", err)
	}
	if _, err := decodeAppleJWKSet(weak); err == nil {
		t.Fatal("weak RSA key was accepted")
	}

	key := testRSAKey(t)
	serialized := appleJWKFromRSA("duplicate", &key.PublicKey)
	duplicate, err := json.Marshal(appleJWKSet{
		Keys: []appleJWK{serialized, serialized},
	})
	if err != nil {
		t.Fatalf("Marshal duplicate keys: %v", err)
	}
	if _, err := decodeAppleJWKSet(duplicate); err == nil {
		t.Fatal("duplicate key ID was accepted")
	}
}

func newTestAppleVerifier(
	t *testing.T,
	keys AppleSigningKeyProvider,
	now time.Time,
) *AppleVerifier {
	t.Helper()
	verifier, err := NewAppleVerifier(AppleVerifierConfig{
		Keys: keys,
		Audiences: map[string]struct{}{
			"com.gochya.companion": {},
			"com.gochya.watch":     {},
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewAppleVerifier: %v", err)
	}
	return verifier
}

func validAppleClaims(now time.Time, nonce string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   appleIssuer,
		"aud":   "com.gochya.companion",
		"exp":   now.Add(5 * time.Minute).Unix(),
		"iat":   now.Add(-time.Minute).Unix(),
		"sub":   "apple-account-123",
		"nonce": nonce,
		"email": "private@example.invalid",
	}
}

func signedAppleToken(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	keyID string,
	claims jwt.MapClaims,
) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	serialized, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return serialized
}

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return key
}

func testAppleNonce(value byte) string {
	return base64.RawURLEncoding.EncodeToString(
		bytes.Repeat([]byte{value}, appleLoginNonceBytes),
	)
}

func appleJWKDocument(
	t *testing.T,
	keyID string,
	key *rsa.PublicKey,
) string {
	t.Helper()
	document, err := json.Marshal(appleJWKSet{
		Keys: []appleJWK{appleJWKFromRSA(keyID, key)},
	})
	if err != nil {
		t.Fatalf("Marshal JWK: %v", err)
	}
	return string(document)
}

func appleJWKFromRSA(keyID string, key *rsa.PublicKey) appleJWK {
	return appleJWK{
		KeyType:   "RSA",
		KeyID:     keyID,
		Use:       "sig",
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		Exponent: base64.RawURLEncoding.EncodeToString(
			big.NewInt(int64(key.E)).Bytes(),
		),
	}
}
