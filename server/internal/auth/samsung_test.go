package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSamsungOIDCTokenClientExchangesCodeWithPKCE(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		if request.Method != http.MethodPost ||
			request.URL.String() != samsungTokenURL ||
			request.Header.Get("Accept") != "application/json" ||
			request.Header.Get("Content-Type") !=
				"application/x-www-form-urlencoded" {
			t.Fatalf("token request = %s %s %#v", request.Method, request.URL, request.Header)
		}
		username, password, ok := request.BasicAuth()
		if !ok ||
			username != "samsung-client" ||
			password != "samsung-secret" {
			t.Fatalf("Basic auth = %q, %q, %v", username, password, ok)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read token request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse token request: %v", err)
		}
		if form.Get("grant_type") != "authorization_code" ||
			form.Get("code") != "authorization-code" ||
			form.Get("redirect_uri") !=
				"https://auth.gochya.example/samsung/callback" ||
			form.Get("code_verifier") != testSamsungOpaqueValue(3) ||
			form.Get("client_secret") != "" {
			t.Fatalf("token form = %#v", form)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"access_token":"discard-me","id_token":"signed-id-token"}`,
			)),
			Request: request,
		}, nil
	})}
	tokenClient, err := NewSamsungOIDCTokenClient(
		SamsungOIDCTokenClientConfig{
			ClientID:     "samsung-client",
			ClientSecret: "samsung-secret",
			HTTPClient:   client,
		},
	)
	if err != nil {
		t.Fatalf("NewSamsungOIDCTokenClient: %v", err)
	}
	identityToken, err := tokenClient.Exchange(
		context.Background(),
		"authorization-code",
		"https://auth.gochya.example/samsung/callback",
		testSamsungOpaqueValue(3),
	)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if identityToken != "signed-id-token" {
		t.Fatalf("identity token = %q", identityToken)
	}
}

func TestSamsungOIDCTokenClientClassifiesProviderResponses(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantInvalid bool
	}{
		{
			name:        "invalid grant",
			status:      http.StatusBadRequest,
			body:        `{"error":"invalid_grant"}`,
			wantInvalid: true,
		},
		{
			name:   "rate limited",
			status: http.StatusTooManyRequests,
			body:   `{"error":"slow_down"}`,
		},
		{
			name:   "server error",
			status: http.StatusBadGateway,
			body:   `{"error":"temporarily_unavailable"}`,
		},
		{
			name:   "malformed success",
			status: http.StatusOK,
			body:   `{"access_token":"missing-id-token"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(
				request *http.Request,
			) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Request:    request,
				}, nil
			})}
			tokenClient, err := NewSamsungOIDCTokenClient(
				SamsungOIDCTokenClientConfig{
					ClientID:     "samsung-client",
					ClientSecret: "samsung-secret",
					HTTPClient:   client,
				},
			)
			if err != nil {
				t.Fatalf("NewSamsungOIDCTokenClient: %v", err)
			}
			_, err = tokenClient.Exchange(
				context.Background(),
				"code",
				"https://auth.gochya.example/samsung/callback",
				testSamsungOpaqueValue(4),
			)
			if test.wantInvalid {
				if !errors.Is(err, ErrIdentityTokenInvalid) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			var unavailable *IdentityProviderUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSamsungOIDCTokenClientClassifiesNetworkFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(
		*http.Request,
	) (*http.Response, error) {
		return nil, &net.DNSError{IsTimeout: true, Err: "timeout"}
	})}
	tokenClient, err := NewSamsungOIDCTokenClient(
		SamsungOIDCTokenClientConfig{
			ClientID:     "samsung-client",
			ClientSecret: "samsung-secret",
			HTTPClient:   client,
		},
	)
	if err != nil {
		t.Fatalf("NewSamsungOIDCTokenClient: %v", err)
	}
	_, err = tokenClient.Exchange(
		context.Background(),
		"code",
		"https://auth.gochya.example/samsung/callback",
		testSamsungOpaqueValue(5),
	)
	var unavailable *IdentityProviderUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestSamsungVerifierAcceptsValidIdentity(t *testing.T) {
	privateKey := testRSAKey(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	nonce := testSamsungOpaqueValue(6)
	keys := &fakeAppleSigningKeyProvider{key: &privateKey.PublicKey}
	verifier := newTestSamsungVerifier(t, keys, now)
	identity, err := verifier.Verify(
		context.Background(),
		signedSamsungToken(
			t,
			privateKey,
			"samsung-test-key",
			validSamsungClaims(now, nonce),
		),
		nonce,
	)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if keys.kid != "samsung-test-key" ||
		identity.Provider != "samsung" ||
		identity.Subject != "samsung-account-123" ||
		identity.DisplayName != "" {
		t.Fatalf("identity = %#v, key ID = %q", identity, keys.kid)
	}
}

func TestSamsungVerifierRejectsInvalidClaims(t *testing.T) {
	privateKey := testRSAKey(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	nonce := testSamsungOpaqueValue(7)
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
				claims["aud"] = "attacker-client"
			},
		},
		{
			name: "wrong nonce",
			mutate: func(claims jwt.MapClaims) {
				claims["nonce"] = testSamsungOpaqueValue(8)
			},
		},
		{
			name: "expired",
			mutate: func(claims jwt.MapClaims) {
				claims["exp"] = now.Unix()
			},
		},
		{
			name: "future issued at",
			mutate: func(claims jwt.MapClaims) {
				claims["iat"] = now.Add(3 * time.Minute).Unix()
			},
		},
		{
			name: "missing issued at",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "iat")
			},
		},
		{
			name: "wrong authorized party",
			mutate: func(claims jwt.MapClaims) {
				claims["aud"] = []string{
					"gochya-samsung-client",
					"another-client",
				}
				claims["azp"] = "another-client"
			},
		},
		{
			name: "missing subject",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "sub")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := validSamsungClaims(now, nonce)
			test.mutate(claims)
			verifier := newTestSamsungVerifier(
				t,
				&fakeAppleSigningKeyProvider{key: &privateKey.PublicKey},
				now,
			)
			_, err := verifier.Verify(
				context.Background(),
				signedSamsungToken(
					t,
					privateKey,
					"samsung-test-key",
					claims,
				),
				nonce,
			)
			if !errors.Is(err, ErrIdentityTokenInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSamsungHTTPKeySetUsesOfficialJWKEndpoint(t *testing.T) {
	privateKey := testRSAKey(t)
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		requests++
		if request.URL.String() != samsungJWKURL {
			t.Fatalf("JWK URL = %q", request.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Cache-Control": []string{"public, max-age=10800"},
			},
			Body: io.NopCloser(strings.NewReader(
				appleJWKDocument(t, "samsung-key", &privateKey.PublicKey),
			)),
			Request: request,
		}, nil
	})}
	keySet, err := NewSamsungHTTPKeySet(HTTPRSAKeySetConfig{
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("NewSamsungHTTPKeySet: %v", err)
	}
	key, err := keySet.SigningKey(context.Background(), "samsung-key")
	if err != nil {
		t.Fatalf("SigningKey: %v", err)
	}
	if key.N.Cmp(privateKey.PublicKey.N) != 0 || requests != 1 {
		t.Fatalf("key mismatch or JWK requests = %d", requests)
	}
}

func newTestSamsungVerifier(
	t *testing.T,
	keys RSASigningKeyProvider,
	now time.Time,
) *SamsungVerifier {
	t.Helper()
	verifier, err := NewSamsungVerifier(SamsungVerifierConfig{
		Keys:     keys,
		ClientID: "gochya-samsung-client",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSamsungVerifier: %v", err)
	}
	return verifier
}

func validSamsungClaims(now time.Time, nonce string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   samsungIssuer,
		"aud":   "gochya-samsung-client",
		"exp":   now.Add(5 * time.Minute).Unix(),
		"iat":   now.Add(-time.Minute).Unix(),
		"sub":   "samsung-account-123",
		"nonce": nonce,
		"email": "ignored@example.invalid",
	}
}

func signedSamsungToken(
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

func testSamsungOpaqueValue(value byte) string {
	return testAppleNonce(value)
}
