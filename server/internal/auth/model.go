package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRefreshTokenInvalid  = errors.New("refresh token is invalid or expired")
	ErrRefreshTokenReused   = errors.New("refresh token reuse detected")
	ErrIdentityTokenInvalid = errors.New("identity token is invalid")
	ErrLoginRequestInvalid  = errors.New("login request is invalid")
)

type TokenPair struct {
	JWT                   string    `json:"jwt"`
	RefreshToken          string    `json:"refreshToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

type Player struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName,omitempty"`
}

type LoginResponse struct {
	TokenPair
	Player Player `json:"player"`
}

type ExternalIdentity struct {
	Provider    string
	Subject     string
	DisplayName string
}

type PlayerCandidate struct {
	ID          string
	Username    string
	DisplayName string
	Identity    ExternalIdentity
	Now         time.Time
}

type RefreshTokenRecord struct {
	ID              string
	FamilyID        string
	PlayerID        string
	DeviceID        string
	TokenHash       [32]byte
	IssuedAt        time.Time
	ExpiresAt       time.Time
	FamilyExpiresAt time.Time
}

type RefreshTokenReplacement struct {
	ID        string
	TokenHash [32]byte
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type RefreshIdentity struct {
	PlayerID        string
	DeviceID        string
	FamilyID        string
	ExpiresAt       time.Time
	FamilyExpiresAt time.Time
}

type RefreshTokenStore interface {
	Create(context.Context, RefreshTokenRecord) error
	Rotate(
		context.Context,
		[32]byte,
		RefreshTokenReplacement,
		time.Time,
	) (RefreshIdentity, error)
	RevokeFamily(context.Context, [32]byte, time.Time) error
}

type SessionManager interface {
	Refresh(context.Context, string) (TokenPair, error)
	Logout(context.Context, string) error
}

type SessionIssuer interface {
	Issue(context.Context, string, string) (TokenPair, error)
}

type IdentityStore interface {
	Resolve(context.Context, PlayerCandidate) (Player, error)
}

type GoogleIdentityVerifier interface {
	Verify(context.Context, string) (ExternalIdentity, error)
}

type GoogleExchanger interface {
	Exchange(context.Context, string, string) (LoginResponse, error)
}

type IdentityProviderUnavailableError struct {
	Cause error
}

func (e *IdentityProviderUnavailableError) Error() string {
	if e.Cause == nil {
		return "identity provider is unavailable"
	}
	return "identity provider is unavailable: " + e.Cause.Error()
}

func (e *IdentityProviderUnavailableError) Unwrap() error {
	return e.Cause
}
