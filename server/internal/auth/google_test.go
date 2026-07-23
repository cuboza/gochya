package auth

import (
	"context"
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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/idtoken"
)

type fakeGoogleIDTokenValidator struct {
	payload   *idtoken.Payload
	err       error
	audiences []string
}

func (validator *fakeGoogleIDTokenValidator) Validate(
	_ context.Context,
	_ string,
	audience string,
) (*idtoken.Payload, error) {
	validator.audiences = append(validator.audiences, audience)
	if validator.err != nil {
		return nil, validator.err
	}
	if validator.payload == nil || validator.payload.Audience != audience {
		return nil, errors.New("audience mismatch")
	}
	return validator.payload, nil
}

func TestGoogleVerifierAcceptsValidIdentity(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	validator := &fakeGoogleIDTokenValidator{payload: validGooglePayload(now)}
	verifier := newTestGoogleVerifier(t, validator, now)

	identity, err := verifier.Verify(context.Background(), "signed-google-token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.Provider != "google" ||
		identity.Subject != "google-account-123" ||
		identity.DisplayName != "Gochya Player" {
		t.Fatalf("identity = %#v", identity)
	}
	if len(validator.audiences) != 2 ||
		validator.audiences[1] != "wear-client.apps.googleusercontent.com" {
		t.Fatalf("validated audiences = %#v", validator.audiences)
	}
}

func TestGoogleVerifierRejectsInvalidClaims(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*idtoken.Payload)
	}{
		{
			name: "wrong issuer",
			mutate: func(payload *idtoken.Payload) {
				payload.Issuer = "https://attacker.example"
			},
		},
		{
			name: "missing subject",
			mutate: func(payload *idtoken.Payload) {
				payload.Subject = ""
			},
		},
		{
			name: "expired",
			mutate: func(payload *idtoken.Payload) {
				payload.Expires = now.Unix()
			},
		},
		{
			name: "future issued at",
			mutate: func(payload *idtoken.Payload) {
				payload.IssuedAt = now.Add(3 * time.Minute).Unix()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validGooglePayload(now)
			test.mutate(payload)
			verifier := newTestGoogleVerifier(
				t,
				&fakeGoogleIDTokenValidator{payload: payload},
				now,
			)
			_, err := verifier.Verify(context.Background(), "signed-google-token")
			if !errors.Is(err, ErrIdentityTokenInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGoogleVerifierClassifiesKeyFetchFailure(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	verifier := newTestGoogleVerifier(
		t,
		&fakeGoogleIDTokenValidator{
			err: &net.DNSError{IsTimeout: true, Err: "timeout"},
		},
		now,
	)
	_, err := verifier.Verify(context.Background(), "signed-google-token")
	var unavailable *IdentityProviderUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestGoogleAPIValidatorVerifiesRS256Token(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":  "https://accounts.google.com",
		"aud":  "wear-client.apps.googleusercontent.com",
		"exp":  now.Add(time.Hour).Unix(),
		"iat":  now.Add(-time.Minute).Unix(),
		"sub":  "google-account-123",
		"name": "Gochya Player",
	})
	token.Header["kid"] = "google-test-key"
	serialized, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	jwk, err := json.Marshal(map[string]any{
		"keys": []map[string]string{
			{
				"alg": "RS256",
				"kid": "google-test-key",
				"kty": "RSA",
				"use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(
					privateKey.PublicKey.N.Bytes(),
				),
				"e": base64.RawURLEncoding.EncodeToString(
					big.NewInt(int64(privateKey.PublicKey.E)).Bytes(),
				),
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal JWK: %v", err)
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		if request.URL.String() != "https://www.googleapis.com/oauth2/v3/certs" {
			t.Fatalf("JWK URL = %q", request.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Cache-Control": []string{"public, max-age=3600"},
			},
			Body:    io.NopCloser(strings.NewReader(string(jwk))),
			Request: request,
		}, nil
	})}
	libraryValidator, err := NewGoogleAPIIDTokenValidator(httpClient)
	if err != nil {
		t.Fatalf("NewGoogleAPIIDTokenValidator: %v", err)
	}
	verifier, err := NewGoogleVerifier(GoogleVerifierConfig{
		Validator: libraryValidator,
		Audiences: map[string]struct{}{
			"wear-client.apps.googleusercontent.com": {},
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGoogleVerifier: %v", err)
	}
	identity, err := verifier.Verify(context.Background(), serialized)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.Subject != "google-account-123" {
		t.Fatalf("identity = %#v", identity)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newTestGoogleVerifier(
	t *testing.T,
	validator GoogleIDTokenValidator,
	now time.Time,
) *GoogleVerifier {
	t.Helper()
	verifier, err := NewGoogleVerifier(GoogleVerifierConfig{
		Validator: validator,
		Audiences: map[string]struct{}{
			"android-client.apps.googleusercontent.com": {},
			"wear-client.apps.googleusercontent.com":    {},
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGoogleVerifier: %v", err)
	}
	return verifier
}

func validGooglePayload(now time.Time) *idtoken.Payload {
	return &idtoken.Payload{
		Issuer:   "https://accounts.google.com",
		Audience: "wear-client.apps.googleusercontent.com",
		Expires:  now.Add(time.Hour).Unix(),
		IssuedAt: now.Add(-time.Minute).Unix(),
		Subject:  "google-account-123",
		Claims: map[string]any{
			"name": "  Gochya Player  ",
		},
	}
}
