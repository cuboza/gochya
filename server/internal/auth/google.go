package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

const (
	maxGoogleIDTokenLength = 16 << 10
	maxGoogleSubjectLength = 255
	maxDisplayNameRunes    = 100
	googleTokenFutureSkew  = 2 * time.Minute
	googleJWKTimeout       = 5 * time.Second
)

type GoogleIDTokenValidator interface {
	Validate(context.Context, string, string) (*idtoken.Payload, error)
}

type GoogleAPIIDTokenValidator struct {
	validator *idtoken.Validator
}

func NewGoogleAPIIDTokenValidator(
	httpClient *http.Client,
) (*GoogleAPIIDTokenValidator, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: googleJWKTimeout}
	}
	validator, err := idtoken.NewValidator(
		context.Background(),
		option.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("create Google API ID-token validator: %w", err)
	}
	return &GoogleAPIIDTokenValidator{validator: validator}, nil
}

func (v *GoogleAPIIDTokenValidator) Validate(
	ctx context.Context,
	token string,
	audience string,
) (*idtoken.Payload, error) {
	if v == nil || v.validator == nil {
		return nil, errors.New("Google API ID-token validator is not configured")
	}
	return v.validator.Validate(ctx, token, audience)
}

type GoogleVerifierConfig struct {
	Validator GoogleIDTokenValidator
	Audiences map[string]struct{}
	Now       func() time.Time
}

type GoogleVerifier struct {
	validator GoogleIDTokenValidator
	audiences []string
	now       func() time.Time
}

func NewGoogleVerifier(config GoogleVerifierConfig) (*GoogleVerifier, error) {
	if config.Validator == nil {
		return nil, errors.New("Google ID-token validator is required")
	}
	if len(config.Audiences) == 0 {
		return nil, errors.New("Google client-ID allowlist is required")
	}
	audiences := make([]string, 0, len(config.Audiences))
	for audience := range config.Audiences {
		audience = strings.TrimSpace(audience)
		if audience == "" || len(audience) > 512 {
			return nil, errors.New("Google client ID is invalid")
		}
		audiences = append(audiences, audience)
	}
	slices.Sort(audiences)
	if config.Now == nil {
		config.Now = time.Now
	}
	return &GoogleVerifier{
		validator: config.Validator,
		audiences: audiences,
		now:       config.Now,
	}, nil
}

func (v *GoogleVerifier) Verify(
	ctx context.Context,
	serialized string,
) (ExternalIdentity, error) {
	if serialized == "" || len(serialized) > maxGoogleIDTokenLength {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	var payload *idtoken.Payload
	var validationErr error
	for _, audience := range v.audiences {
		candidate, err := v.validator.Validate(ctx, serialized, audience)
		if err == nil {
			payload = candidate
			break
		}
		if googleValidationUnavailable(err) {
			return ExternalIdentity{}, &IdentityProviderUnavailableError{Cause: err}
		}
		validationErr = err
	}
	if payload == nil {
		return ExternalIdentity{}, fmt.Errorf(
			"%w: %v",
			ErrIdentityTokenInvalid,
			validationErr,
		)
	}
	if payload.Issuer != "accounts.google.com" &&
		payload.Issuer != "https://accounts.google.com" {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	subject := strings.TrimSpace(payload.Subject)
	if subject == "" || len(subject) > maxGoogleSubjectLength {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	nowTime := v.now().UTC()
	now := nowTime.Unix()
	if payload.Expires <= now ||
		payload.IssuedAt <= 0 ||
		payload.IssuedAt > nowTime.Add(googleTokenFutureSkew).Unix() {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	displayName := googleDisplayName(payload.Claims["name"])
	return ExternalIdentity{
		Provider:    "google",
		Subject:     subject,
		DisplayName: displayName,
	}, nil
}

func googleDisplayName(value any) string {
	name, ok := value.(string)
	if !ok || !utf8.ValidString(name) {
		return ""
	}
	name = strings.TrimSpace(name)
	runes := []rune(name)
	if len(runes) > maxDisplayNameRunes {
		runes = runes[:maxDisplayNameRunes]
	}
	return string(runes)
}

func googleValidationUnavailable(err error) bool {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	return strings.Contains(err.Error(), "unable to retrieve cert")
}
