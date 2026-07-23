package dojo

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type failingOAuth2TokenSource struct{}

func (failingOAuth2TokenSource) Token() (*oauth2.Token, error) {
	return nil, errors.New("credential failure")
}

func TestOAuth2PlayIntegrityAccessTokenSource(t *testing.T) {
	source, err := NewOAuth2PlayIntegrityAccessTokenSource(
		oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: "google-access-token",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Hour),
		}),
	)
	if err != nil {
		t.Fatalf("NewOAuth2PlayIntegrityAccessTokenSource: %v", err)
	}
	token, err := source.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if token != "google-access-token" {
		t.Fatalf("access token = %q", token)
	}
}

func TestOAuth2PlayIntegrityAccessTokenSourcePropagatesFailure(t *testing.T) {
	source, err := NewOAuth2PlayIntegrityAccessTokenSource(failingOAuth2TokenSource{})
	if err != nil {
		t.Fatalf("NewOAuth2PlayIntegrityAccessTokenSource: %v", err)
	}
	if _, err := source.AccessToken(context.Background()); err == nil {
		t.Fatal("credential failure was ignored")
	}
}
