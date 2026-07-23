package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultAccessTokenTTL   = 15 * time.Minute
	defaultRefreshTokenTTL  = 30 * 24 * time.Hour
	defaultMaxSessionTTL    = 90 * 24 * time.Hour
	refreshTokenBytes       = 32
	maxSerializedRefreshLen = 128
)

type ServiceConfig struct {
	Store         RefreshTokenStore
	KeyID         string
	PrivateKey    ed25519.PrivateKey
	Issuer        string
	Audience      string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	MaxSessionTTL time.Duration
	Now           func() time.Time
	Random        io.Reader
}

type Service struct {
	store         RefreshTokenStore
	keyID         string
	privateKey    ed25519.PrivateKey
	issuer        string
	audience      string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	maxSessionTTL time.Duration
	now           func() time.Time
	random        io.Reader
}

type accessTokenClaims struct {
	TokenUse string `json:"token_use"`
	jwt.RegisteredClaims
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil {
		return nil, errors.New("refresh-token store is required")
	}
	if strings.TrimSpace(config.KeyID) == "" || len(config.KeyID) > 128 {
		return nil, errors.New("JWT signing key ID is invalid")
	}
	if len(config.PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf(
			"JWT signing key must contain %d bytes",
			ed25519.PrivateKeySize,
		)
	}
	if strings.TrimSpace(config.Issuer) == "" ||
		strings.TrimSpace(config.Audience) == "" {
		return nil, errors.New("JWT issuer and audience are required")
	}
	if config.AccessTTL == 0 {
		config.AccessTTL = defaultAccessTokenTTL
	}
	if config.RefreshTTL == 0 {
		config.RefreshTTL = defaultRefreshTokenTTL
	}
	if config.MaxSessionTTL == 0 {
		config.MaxSessionTTL = defaultMaxSessionTTL
	}
	if config.AccessTTL <= 0 || config.AccessTTL > time.Hour {
		return nil, errors.New("access-token TTL must be between zero and one hour")
	}
	if config.RefreshTTL <= 0 || config.RefreshTTL > 90*24*time.Hour {
		return nil, errors.New("refresh-token TTL must be between zero and 90 days")
	}
	if config.MaxSessionTTL < config.RefreshTTL ||
		config.MaxSessionTTL > 365*24*time.Hour {
		return nil, errors.New(
			"maximum session TTL must cover refresh TTL and not exceed 365 days",
		)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &Service{
		store:         config.Store,
		keyID:         config.KeyID,
		privateKey:    bytes.Clone(config.PrivateKey),
		issuer:        config.Issuer,
		audience:      config.Audience,
		accessTTL:     config.AccessTTL,
		refreshTTL:    config.RefreshTTL,
		maxSessionTTL: config.MaxSessionTTL,
		now:           config.Now,
		random:        config.Random,
	}, nil
}

func (s *Service) Issue(
	ctx context.Context,
	playerID string,
	deviceID string,
) (TokenPair, error) {
	if err := validateUUID(playerID); err != nil {
		return TokenPair{}, fmt.Errorf("issue session: %w", err)
	}
	now := s.now().UTC()
	refresh, hash, err := s.newRefreshToken()
	if err != nil {
		return TokenPair{}, fmt.Errorf("generate refresh token: %w", err)
	}
	refreshID, err := randomUUID(s.random)
	if err != nil {
		return TokenPair{}, fmt.Errorf("generate refresh-token ID: %w", err)
	}
	accessID, err := randomUUID(s.random)
	if err != nil {
		return TokenPair{}, fmt.Errorf("generate access-token ID: %w", err)
	}
	access, accessExpiresAt, err := s.signAccess(playerID, accessID, now)
	if err != nil {
		return TokenPair{}, err
	}
	familyExpiresAt := now.Add(s.maxSessionTTL)
	refreshExpiresAt := minTime(now.Add(s.refreshTTL), familyExpiresAt)
	if err := s.store.Create(ctx, RefreshTokenRecord{
		ID:              refreshID,
		FamilyID:        refreshID,
		PlayerID:        playerID,
		DeviceID:        strings.TrimSpace(deviceID),
		TokenHash:       hash,
		IssuedAt:        now,
		ExpiresAt:       refreshExpiresAt,
		FamilyExpiresAt: familyExpiresAt,
	}); err != nil {
		return TokenPair{}, fmt.Errorf("store refresh token: %w", err)
	}
	return TokenPair{
		JWT:                   access,
		RefreshToken:          refresh,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}

func (s *Service) Refresh(
	ctx context.Context,
	currentToken string,
) (TokenPair, error) {
	currentHash, err := refreshTokenHash(currentToken)
	if err != nil {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	now := s.now().UTC()
	replacementToken, replacementHash, err := s.newRefreshToken()
	if err != nil {
		return TokenPair{}, fmt.Errorf("generate refresh token: %w", err)
	}
	replacementID, err := randomUUID(s.random)
	if err != nil {
		return TokenPair{}, fmt.Errorf("generate refresh-token ID: %w", err)
	}
	accessID, err := randomUUID(s.random)
	if err != nil {
		return TokenPair{}, fmt.Errorf("generate access-token ID: %w", err)
	}
	identity, err := s.store.Rotate(
		ctx,
		currentHash,
		RefreshTokenReplacement{
			ID:        replacementID,
			TokenHash: replacementHash,
			IssuedAt:  now,
			ExpiresAt: now.Add(s.refreshTTL),
		},
		now,
	)
	if err != nil {
		return TokenPair{}, err
	}
	access, accessExpiresAt, err := s.signAccess(identity.PlayerID, accessID, now)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		JWT:                   access,
		RefreshToken:          replacementToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: identity.ExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	hash, err := refreshTokenHash(token)
	if err != nil {
		return nil
	}
	if err := s.store.RevokeFamily(ctx, hash, s.now().UTC()); err != nil {
		return fmt.Errorf("revoke refresh-token family: %w", err)
	}
	return nil
}

func (s *Service) signAccess(
	playerID string,
	tokenID string,
	now time.Time,
) (string, time.Time, error) {
	expiresAt := now.Add(s.accessTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, accessTokenClaims{
		TokenUse: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   playerID,
			Audience:  jwt.ClaimStrings{s.audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        tokenID,
		},
	})
	token.Header["kid"] = s.keyID
	serialized, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return serialized, expiresAt, nil
}

func (s *Service) newRefreshToken() (string, [32]byte, error) {
	value := make([]byte, refreshTokenBytes)
	if _, err := io.ReadFull(s.random, value); err != nil {
		return "", [32]byte{}, err
	}
	serialized := base64.RawURLEncoding.EncodeToString(value)
	return serialized, sha256.Sum256([]byte(serialized)), nil
}

func refreshTokenHash(serialized string) ([32]byte, error) {
	if serialized == "" || len(serialized) > maxSerializedRefreshLen {
		return [32]byte{}, ErrRefreshTokenInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(serialized)
	if err != nil || len(decoded) != refreshTokenBytes ||
		base64.RawURLEncoding.EncodeToString(decoded) != serialized {
		return [32]byte{}, ErrRefreshTokenInvalid
	}
	return sha256.Sum256([]byte(serialized)), nil
}

func randomUUID(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}

func validateUUID(value string) error {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return errors.New("player ID must be a UUID")
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded := make([]byte, 16)
	if _, err := hex.Decode(decoded, []byte(compact)); err != nil {
		return errors.New("player ID must be a UUID")
	}
	return nil
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
