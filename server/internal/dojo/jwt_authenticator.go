package dojo

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultJWTLeeway      = 30 * time.Second
	defaultJWTMaxLifetime = 15 * time.Minute
	maxBearerTokenLength  = 8192
)

type JWTAuthenticatorConfig struct {
	Issuer      string
	Audience    string
	PublicKeys  map[string]ed25519.PublicKey
	Leeway      time.Duration
	MaxLifetime time.Duration
	Now         func() time.Time
}

// JWTAuthenticator verifies short-lived Ed25519 access tokens and supports
// explicit key rotation through the protected JWT kid header.
type JWTAuthenticator struct {
	issuer      string
	audience    string
	publicKeys  map[string]ed25519.PublicKey
	leeway      time.Duration
	maxLifetime time.Duration
	now         func() time.Time
}

var _ PlayerAuthenticator = (*JWTAuthenticator)(nil)

type accessTokenClaims struct {
	TokenUse string `json:"token_use"`
	jwt.RegisteredClaims
}

func NewJWTAuthenticator(config JWTAuthenticatorConfig) (*JWTAuthenticator, error) {
	if strings.TrimSpace(config.Issuer) == "" || strings.TrimSpace(config.Audience) == "" {
		return nil, errors.New("JWT issuer and audience are required")
	}
	if len(config.PublicKeys) == 0 {
		return nil, errors.New("at least one JWT public key is required")
	}
	publicKeys := make(map[string]ed25519.PublicKey, len(config.PublicKeys))
	for keyID, publicKey := range config.PublicKeys {
		if strings.TrimSpace(keyID) == "" || len(keyID) > 128 {
			return nil, errors.New("JWT key ID is invalid")
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("JWT public key %q has %d bytes", keyID, len(publicKey))
		}
		publicKeys[keyID] = bytes.Clone(publicKey)
	}
	if config.Leeway == 0 {
		config.Leeway = defaultJWTLeeway
	}
	if config.Leeway < 0 || config.Leeway > 2*time.Minute {
		return nil, errors.New("JWT leeway must be between zero and two minutes")
	}
	if config.MaxLifetime == 0 {
		config.MaxLifetime = defaultJWTMaxLifetime
	}
	if config.MaxLifetime <= 0 || config.MaxLifetime > time.Hour {
		return nil, errors.New("JWT maximum lifetime must be between zero and one hour")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &JWTAuthenticator{
		issuer:      config.Issuer,
		audience:    config.Audience,
		publicKeys:  publicKeys,
		leeway:      config.Leeway,
		maxLifetime: config.MaxLifetime,
		now:         config.Now,
	}, nil
}

func (a *JWTAuthenticator) Authenticate(request *http.Request) (string, error) {
	serialized, err := bearerToken(request.Header.Get("Authorization"))
	if err != nil {
		return "", unauthenticatedError(err)
	}
	claims := &accessTokenClaims{}
	token, err := jwt.ParseWithClaims(
		serialized,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodEdDSA {
				return nil, errors.New("JWT signing method is not EdDSA")
			}
			keyID, ok := token.Header["kid"].(string)
			if !ok || keyID == "" {
				return nil, errors.New("JWT key ID is missing")
			}
			publicKey, ok := a.publicKeys[keyID]
			if !ok {
				return nil, errors.New("JWT key ID is unknown")
			}
			return publicKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(a.issuer),
		jwt.WithAudience(a.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(a.leeway),
		jwt.WithTimeFunc(a.now),
		jwt.WithStrictDecoding(),
	)
	if err != nil || token == nil || !token.Valid {
		if err == nil {
			err = errors.New("JWT is invalid")
		}
		return "", unauthenticatedError(err)
	}
	if claims.TokenUse != "access" {
		return "", unauthenticatedError(errors.New("JWT is not an access token"))
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.ID == "" {
		return "", unauthenticatedError(errors.New("JWT is missing required claims"))
	}
	if claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) <= 0 ||
		claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) > a.maxLifetime {
		return "", unauthenticatedError(errors.New("JWT lifetime is invalid"))
	}
	if err := validateUUID(claims.Subject); err != nil {
		return "", unauthenticatedError(errors.New("JWT subject is invalid"))
	}
	return claims.Subject, nil
}

func bearerToken(header string) (string, error) {
	if len(header) > len("Bearer ")+maxBearerTokenLength {
		return "", errors.New("authorization header is too large")
	}
	parts := strings.Split(header, " ")
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return "", errors.New("authorization header must contain one Bearer token")
	}
	return parts[1], nil
}

func unauthenticatedError(cause error) *Error {
	return &Error{
		Code:       "unauthenticated",
		Message:    "valid access token is required",
		HTTPStatus: http.StatusUnauthorized,
		Cause:      cause,
	}
}
