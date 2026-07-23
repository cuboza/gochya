package dojo

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const playIntegrityOAuthScope = "https://www.googleapis.com/auth/playintegrity"

type OAuth2PlayIntegrityAccessTokenSource struct {
	source oauth2.TokenSource
}

var _ PlayIntegrityAccessTokenSource = (*OAuth2PlayIntegrityAccessTokenSource)(nil)

func NewOAuth2PlayIntegrityAccessTokenSource(
	source oauth2.TokenSource,
) (*OAuth2PlayIntegrityAccessTokenSource, error) {
	if source == nil {
		return nil, errors.New("OAuth2 token source is required")
	}
	return &OAuth2PlayIntegrityAccessTokenSource{
		source: oauth2.ReuseTokenSource(nil, source),
	}, nil
}

// NewGoogleDefaultPlayIntegrityAccessTokenSource uses Application Default
// Credentials and supports attached service accounts and Workload Identity.
func NewGoogleDefaultPlayIntegrityAccessTokenSource(
	ctx context.Context,
) (*OAuth2PlayIntegrityAccessTokenSource, error) {
	source, err := google.DefaultTokenSource(ctx, playIntegrityOAuthScope)
	if err != nil {
		return nil, fmt.Errorf("load Google Application Default Credentials: %w", err)
	}
	return NewOAuth2PlayIntegrityAccessTokenSource(source)
}

func (s *OAuth2PlayIntegrityAccessTokenSource) AccessToken(
	ctx context.Context,
) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	token, err := s.source.Token()
	if err != nil {
		return "", fmt.Errorf("obtain Google OAuth2 access token: %w", err)
	}
	if token == nil || !token.Valid() || token.AccessToken == "" {
		return "", errors.New("Google OAuth2 access token is invalid")
	}
	return token.AccessToken, nil
}
