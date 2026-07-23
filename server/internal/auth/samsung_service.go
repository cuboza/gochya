package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const (
	samsungLoginProvider        = "samsung"
	samsungOpaqueValueBytes     = 32
	defaultSamsungLoginStateTTL = 5 * time.Minute
	maxSamsungLoginStateTTL     = 15 * time.Minute
)

type SamsungExchangeServiceConfig struct {
	ClientID     string
	RedirectURIs map[string]struct{}
	Tokens       SamsungCodeTokenExchanger
	Verifier     SamsungIdentityVerifier
	Nonces       LoginNonceStore
	Players      IdentityStore
	Sessions     SessionIssuer
	StateTTL     time.Duration
	Now          func() time.Time
	Random       io.Reader
}

type SamsungExchangeService struct {
	clientID     string
	redirectURIs map[string]struct{}
	tokens       SamsungCodeTokenExchanger
	verifier     SamsungIdentityVerifier
	nonces       LoginNonceStore
	players      IdentityStore
	sessions     SessionIssuer
	stateTTL     time.Duration
	now          func() time.Time
	random       io.Reader
}

func NewSamsungExchangeService(
	config SamsungExchangeServiceConfig,
) (*SamsungExchangeService, error) {
	if !validSamsungClientID(config.ClientID) {
		return nil, errors.New("Samsung OIDC client ID is invalid")
	}
	if len(config.RedirectURIs) == 0 {
		return nil, errors.New("Samsung redirect-URI allowlist is required")
	}
	redirectURIs := make(map[string]struct{}, len(config.RedirectURIs))
	for redirectURI := range config.RedirectURIs {
		redirectURI = strings.TrimSpace(redirectURI)
		if !validSamsungRedirectURI(redirectURI) {
			return nil, errors.New("Samsung redirect URI is invalid")
		}
		redirectURIs[redirectURI] = struct{}{}
	}
	if config.Tokens == nil ||
		config.Verifier == nil ||
		config.Nonces == nil ||
		config.Players == nil ||
		config.Sessions == nil {
		return nil, errors.New(
			"Samsung token client, verifier, nonce store, identity store and session issuer are required",
		)
	}
	if config.StateTTL == 0 {
		config.StateTTL = defaultSamsungLoginStateTTL
	}
	if config.StateTTL <= 0 || config.StateTTL > maxSamsungLoginStateTTL {
		return nil, errors.New(
			"Samsung login state TTL must be between zero and 15 minutes",
		)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &SamsungExchangeService{
		clientID:     config.ClientID,
		redirectURIs: redirectURIs,
		tokens:       config.Tokens,
		verifier:     config.Verifier,
		nonces:       config.Nonces,
		players:      config.Players,
		sessions:     config.Sessions,
		stateTTL:     config.StateTTL,
		now:          config.Now,
		random:       config.Random,
	}, nil
}

func (s *SamsungExchangeService) Preflight(
	ctx context.Context,
	redirectURI string,
) (SamsungPreflightResponse, error) {
	redirectURI = strings.TrimSpace(redirectURI)
	if _, allowed := s.redirectURIs[redirectURI]; !allowed {
		return SamsungPreflightResponse{}, ErrLoginRequestInvalid
	}
	state, err := randomSamsungOpaqueValue(s.random)
	if err != nil {
		return SamsungPreflightResponse{}, fmt.Errorf(
			"generate Samsung login state: %w",
			err,
		)
	}
	nonce, err := randomSamsungOpaqueValue(s.random)
	if err != nil {
		return SamsungPreflightResponse{}, fmt.Errorf(
			"generate Samsung login nonce: %w",
			err,
		)
	}
	codeVerifier, err := randomSamsungOpaqueValue(s.random)
	if err != nil {
		return SamsungPreflightResponse{}, fmt.Errorf(
			"generate Samsung PKCE verifier: %w",
			err,
		)
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.stateTTL)
	if err := s.nonces.Create(ctx, LoginNonceRecord{
		Provider: samsungLoginProvider,
		Nonce:    state,
		Binding: samsungLoginBinding(
			codeVerifier,
			redirectURI,
			nonce,
		),
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}); err != nil {
		return SamsungPreflightResponse{}, fmt.Errorf(
			"store Samsung login state: %w",
			err,
		)
	}
	authorization, err := url.Parse(samsungAuthorizationURL)
	if err != nil {
		return SamsungPreflightResponse{}, fmt.Errorf(
			"parse Samsung authorization URL: %w",
			err,
		)
	}
	challengeDigest := sha256.Sum256([]byte(codeVerifier))
	query := authorization.Query()
	query.Set("client_id", s.clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "openid")
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set(
		"code_challenge",
		base64.RawURLEncoding.EncodeToString(challengeDigest[:]),
	)
	query.Set("code_challenge_method", "S256")
	authorization.RawQuery = query.Encode()
	return SamsungPreflightResponse{
		AuthorizationURL: authorization.String(),
		State:            state,
		Nonce:            nonce,
		CodeVerifier:     codeVerifier,
		ExpiresAt:        expiresAt,
	}, nil
}

func (s *SamsungExchangeService) Exchange(
	ctx context.Context,
	code string,
	state string,
	nonce string,
	codeVerifier string,
	redirectURI string,
	deviceID string,
) (LoginResponse, error) {
	redirectURI = strings.TrimSpace(redirectURI)
	if code == "" ||
		len(code) > maxSamsungAuthorizationCode ||
		!validSamsungOpaqueValue(state) ||
		!validSamsungOpaqueValue(nonce) ||
		!validSamsungOpaqueValue(codeVerifier) ||
		len(deviceID) > maxAuthDeviceIDLength {
		return LoginResponse{}, ErrLoginRequestInvalid
	}
	if _, allowed := s.redirectURIs[redirectURI]; !allowed {
		return LoginResponse{}, ErrLoginRequestInvalid
	}
	now := s.now().UTC()
	if err := s.nonces.Consume(
		ctx,
		samsungLoginProvider,
		state,
		samsungLoginBinding(codeVerifier, redirectURI, nonce),
		now,
	); err != nil {
		return LoginResponse{}, err
	}
	identityToken, err := s.tokens.Exchange(
		ctx,
		code,
		redirectURI,
		codeVerifier,
	)
	if err != nil {
		return LoginResponse{}, err
	}
	identity, err := s.verifier.Verify(ctx, identityToken, nonce)
	if err != nil {
		return LoginResponse{}, err
	}
	if identity.Provider != samsungLoginProvider ||
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
		Now:         now,
	})
	if err != nil {
		return LoginResponse{}, fmt.Errorf("resolve Samsung player: %w", err)
	}
	pair, err := s.sessions.Issue(ctx, player.ID, strings.TrimSpace(deviceID))
	if err != nil {
		return LoginResponse{}, fmt.Errorf("issue Samsung session: %w", err)
	}
	return LoginResponse{TokenPair: pair, Player: player}, nil
}

func randomSamsungOpaqueValue(reader io.Reader) (string, error) {
	value := make([]byte, samsungOpaqueValueBytes)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validSamsungOpaqueValue(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil &&
		len(decoded) == samsungOpaqueValueBytes &&
		base64.RawURLEncoding.EncodeToString(decoded) == value
}

func samsungLoginBinding(
	codeVerifier string,
	redirectURI string,
	nonce string,
) string {
	return codeVerifier + "\x00" + redirectURI + "\x00" + nonce
}
