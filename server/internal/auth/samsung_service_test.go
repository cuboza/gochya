package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"testing"
	"time"
)

type fakeSamsungCodeTokenExchanger struct {
	identityToken string
	err           error
	code          string
	redirectURI   string
	codeVerifier  string
}

func (exchange *fakeSamsungCodeTokenExchanger) Exchange(
	_ context.Context,
	code string,
	redirectURI string,
	codeVerifier string,
) (string, error) {
	exchange.code = code
	exchange.redirectURI = redirectURI
	exchange.codeVerifier = codeVerifier
	return exchange.identityToken, exchange.err
}

type fakeSamsungIdentityVerifier struct {
	identity ExternalIdentity
	err      error
	token    string
	nonce    string
}

func (verifier *fakeSamsungIdentityVerifier) Verify(
	_ context.Context,
	token string,
	nonce string,
) (ExternalIdentity, error) {
	verifier.token = token
	verifier.nonce = nonce
	return verifier.identity, verifier.err
}

func TestSamsungExchangeBuildsBoundAuthorizationAndIssuesSession(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	random := make([]byte, samsungOpaqueValueBytes*3+16)
	for index := range random {
		random[index] = byte(index + 1)
	}
	nonces := &fakeLoginNonceStore{}
	tokens := &fakeSamsungCodeTokenExchanger{
		identityToken: "samsung-id-token",
	}
	verifier := &fakeSamsungIdentityVerifier{identity: ExternalIdentity{
		Provider: "samsung",
		Subject:  "samsung-account-123",
	}}
	players := &fakeIdentityStore{player: Player{
		ID:       "77777777-7777-4777-8777-777777777777",
		Username: "samsung_stable",
	}}
	sessions := &fakeSessionIssuer{pair: TokenPair{JWT: "access"}}
	service := newTestSamsungExchangeService(
		t,
		nonces,
		tokens,
		verifier,
		players,
		sessions,
		now,
		bytes.NewReader(random),
	)
	redirectURI := "https://auth.gochya.example/samsung/callback"
	preflight, err := service.Preflight(context.Background(), redirectURI)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if !validSamsungOpaqueValue(preflight.State) ||
		!validSamsungOpaqueValue(preflight.Nonce) ||
		!validSamsungOpaqueValue(preflight.CodeVerifier) ||
		preflight.ExpiresAt != now.Add(defaultSamsungLoginStateTTL) {
		t.Fatalf("preflight = %#v", preflight)
	}
	authorization, err := url.Parse(preflight.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	query := authorization.Query()
	challenge := sha256.Sum256([]byte(preflight.CodeVerifier))
	if authorization.Scheme+"://"+authorization.Host+authorization.Path !=
		samsungAuthorizationURL ||
		query.Get("client_id") != "gochya-samsung-client" ||
		query.Get("response_type") != "code" ||
		query.Get("redirect_uri") != redirectURI ||
		query.Get("scope") != "openid" ||
		query.Get("state") != preflight.State ||
		query.Get("nonce") != preflight.Nonce ||
		query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") !=
			base64.RawURLEncoding.EncodeToString(challenge[:]) {
		t.Fatalf("authorization URL = %q", preflight.AuthorizationURL)
	}
	expectedBinding := samsungLoginBinding(
		preflight.CodeVerifier,
		redirectURI,
		preflight.Nonce,
	)
	if nonces.record.Provider != samsungLoginProvider ||
		nonces.record.Nonce != preflight.State ||
		nonces.record.Binding != expectedBinding {
		t.Fatalf("stored state = %#v", nonces.record)
	}

	response, err := service.Exchange(
		context.Background(),
		"authorization-code",
		preflight.State,
		preflight.Nonce,
		preflight.CodeVerifier,
		redirectURI,
		" phone-1 ",
	)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if !nonces.used ||
		tokens.code != "authorization-code" ||
		tokens.redirectURI != redirectURI ||
		tokens.codeVerifier != preflight.CodeVerifier ||
		verifier.token != "samsung-id-token" ||
		verifier.nonce != preflight.Nonce ||
		players.candidate.Username != "samsung_f5cac2b7e3c6b1695c71d8be" ||
		sessions.deviceID != "phone-1" ||
		response.JWT != "access" {
		t.Fatalf(
			"exchange state: nonce=%#v tokens=%#v verifier=%#v candidate=%#v session=%#v response=%#v",
			nonces,
			tokens,
			verifier,
			players.candidate,
			sessions,
			response,
		)
	}
}

func TestSamsungExchangeRejectsTamperedPKCEBinding(t *testing.T) {
	now := time.Now().UTC()
	nonces := &fakeLoginNonceStore{}
	tokens := &fakeSamsungCodeTokenExchanger{}
	service := newTestSamsungExchangeService(
		t,
		nonces,
		tokens,
		&fakeSamsungIdentityVerifier{},
		&fakeIdentityStore{},
		&fakeSessionIssuer{},
		now,
		bytes.NewReader(bytes.Repeat([]byte{1}, 128)),
	)
	preflight, err := service.Preflight(
		context.Background(),
		"https://auth.gochya.example/samsung/callback",
	)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	_, err = service.Exchange(
		context.Background(),
		"code",
		preflight.State,
		preflight.Nonce,
		testSamsungOpaqueValue(99),
		"https://auth.gochya.example/samsung/callback",
		"",
	)
	if !errors.Is(err, ErrLoginNonceInvalid) || tokens.code != "" {
		t.Fatalf("error = %v, token exchange code = %q", err, tokens.code)
	}
}

func TestSamsungPreflightRejectsUnregisteredRedirectURI(t *testing.T) {
	service := newTestSamsungExchangeService(
		t,
		&fakeLoginNonceStore{},
		&fakeSamsungCodeTokenExchanger{},
		&fakeSamsungIdentityVerifier{},
		&fakeIdentityStore{},
		&fakeSessionIssuer{},
		time.Now().UTC(),
		bytes.NewReader(bytes.Repeat([]byte{1}, 128)),
	)
	_, err := service.Preflight(
		context.Background(),
		"https://attacker.example/callback",
	)
	if !errors.Is(err, ErrLoginRequestInvalid) {
		t.Fatalf("error = %v", err)
	}
}

func TestSamsungExchangeServiceRejectsInsecureRedirectURI(t *testing.T) {
	_, err := NewSamsungExchangeService(SamsungExchangeServiceConfig{
		ClientID: "gochya-samsung-client",
		RedirectURIs: map[string]struct{}{
			"http://auth.gochya.example/samsung/callback": {},
		},
		Tokens:   &fakeSamsungCodeTokenExchanger{},
		Verifier: &fakeSamsungIdentityVerifier{},
		Nonces:   &fakeLoginNonceStore{},
		Players:  &fakeIdentityStore{},
		Sessions: &fakeSessionIssuer{},
	})
	if err == nil {
		t.Fatal("insecure Samsung redirect URI was accepted")
	}
}

func newTestSamsungExchangeService(
	t *testing.T,
	nonces LoginNonceStore,
	tokens SamsungCodeTokenExchanger,
	verifier SamsungIdentityVerifier,
	players IdentityStore,
	sessions SessionIssuer,
	now time.Time,
	random *bytes.Reader,
) *SamsungExchangeService {
	t.Helper()
	service, err := NewSamsungExchangeService(SamsungExchangeServiceConfig{
		ClientID: "gochya-samsung-client",
		RedirectURIs: map[string]struct{}{
			"https://auth.gochya.example/samsung/callback": {},
		},
		Tokens:   tokens,
		Verifier: verifier,
		Nonces:   nonces,
		Players:  players,
		Sessions: sessions,
		Now:      func() time.Time { return now },
		Random:   random,
	})
	if err != nil {
		t.Fatalf("NewSamsungExchangeService: %v", err)
	}
	return service
}
