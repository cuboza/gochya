package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRefreshTokenInvalid = errors.New("refresh token is invalid or expired")
	ErrRefreshTokenReused  = errors.New("refresh token reuse detected")
)

type TokenPair struct {
	JWT                   string    `json:"jwt"`
	RefreshToken          string    `json:"refreshToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
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
