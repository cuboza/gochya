package dojo

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testJWTIssuer   = "https://auth.gochya.test"
	testJWTAudience = "gochya-api"
	testJWTSubject  = "77777777-7777-4777-8777-777777777777"
)

func TestJWTAuthenticatorAcceptsValidAccessToken(t *testing.T) {
	authenticator, privateKey, now := newTestJWTAuthenticator(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/dojo/preflight", nil)
	request.Header.Set("Authorization", "Bearer "+signedAccessToken(
		t,
		privateKey,
		now,
		func(*accessTokenClaims) {},
	))

	playerID, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if playerID != testJWTSubject {
		t.Fatalf("player ID = %q", playerID)
	}
}

func TestJWTAuthenticatorRejectsInvalidTokens(t *testing.T) {
	authenticator, privateKey, now := newTestJWTAuthenticator(t)
	tests := []struct {
		name   string
		header func(*testing.T) string
	}{
		{
			name: "missing bearer",
			header: func(*testing.T) string {
				return ""
			},
		},
		{
			name: "expired",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute))
				})
			},
		},
		{
			name: "wrong issuer",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.Issuer = "https://attacker.example"
				})
			},
		},
		{
			name: "wrong audience",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.Audience = jwt.ClaimStrings{"another-api"}
				})
			},
		},
		{
			name: "refresh token",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.TokenUse = "refresh"
				})
			},
		},
		{
			name: "missing issued at",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.IssuedAt = nil
				})
			},
		},
		{
			name: "missing token id",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.ID = ""
				})
			},
		},
		{
			name: "lifetime too long",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour))
				})
			},
		},
		{
			name: "invalid subject",
			header: func(t *testing.T) string {
				return "Bearer " + signedAccessToken(t, privateKey, now, func(claims *accessTokenClaims) {
					claims.Subject = "player-1"
				})
			},
		},
		{
			name: "unknown key",
			header: func(t *testing.T) string {
				token := validAccessToken(now)
				token.Header["kid"] = "unknown"
				serialized, err := token.SignedString(privateKey)
				if err != nil {
					t.Fatalf("SignedString: %v", err)
				}
				return "Bearer " + serialized
			},
		},
		{
			name: "algorithm substitution",
			header: func(t *testing.T) string {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, validAccessToken(now).Claims)
				token.Header["kid"] = "test-key"
				serialized, err := token.SignedString([]byte("not-an-ed25519-key"))
				if err != nil {
					t.Fatalf("SignedString: %v", err)
				}
				return "Bearer " + serialized
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", nil)
			request.Header.Set("Authorization", test.header(t))
			_, err := authenticator.Authenticate(request)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != "unauthenticated" ||
				apiErr.HTTPStatus != http.StatusUnauthorized {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestJWTAuthenticatorClonesPublicKeys(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	authenticator, err := NewJWTAuthenticator(JWTAuthenticatorConfig{
		Issuer:     testJWTIssuer,
		Audience:   testJWTAudience,
		PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey},
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	publicKey[0] ^= 0xff
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(
		"Authorization",
		"Bearer "+signedAccessToken(t, privateKey, now, func(*accessTokenClaims) {}),
	)
	if _, err := authenticator.Authenticate(request); err != nil {
		t.Fatalf("Authenticate after caller key mutation: %v", err)
	}
}

func newTestJWTAuthenticator(
	t *testing.T,
) (*JWTAuthenticator, ed25519.PrivateKey, time.Time) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 10)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	authenticator, err := NewJWTAuthenticator(JWTAuthenticatorConfig{
		Issuer:     testJWTIssuer,
		Audience:   testJWTAudience,
		PublicKeys: map[string]ed25519.PublicKey{"test-key": privateKey.Public().(ed25519.PublicKey)},
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	return authenticator, privateKey, now
}

func validAccessToken(now time.Time) *jwt.Token {
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, accessTokenClaims{
		TokenUse: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testJWTIssuer,
			Subject:   testJWTSubject,
			Audience:  jwt.ClaimStrings{testJWTAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        "88888888-8888-4888-8888-888888888888",
		},
	})
}

func signedAccessToken(
	t *testing.T,
	privateKey ed25519.PrivateKey,
	now time.Time,
	mutate func(*accessTokenClaims),
) string {
	t.Helper()
	claims := validAccessToken(now).Claims.(accessTokenClaims)
	mutate(&claims)
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = "test-key"
	serialized, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return serialized
}
