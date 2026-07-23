package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
)

const (
	samsungIssuer                = "https://account.samsung.com/iam"
	samsungAuthorizationURL      = samsungIssuer + "/oidc/authorize"
	samsungTokenURL              = "https://api.account.samsung.com/auth/oidc/token"
	samsungJWKURL                = "https://account.samsung.com/.well-known/jwks"
	maxSamsungIDTokenLength      = 16 << 10
	maxSamsungAuthorizationCode  = 4 << 10
	maxSamsungTokenResponseBytes = 64 << 10
	maxSamsungSubjectLength      = 255
	maxSamsungClientIDLength     = 512
	maxSamsungClientSecretLength = 4 << 10
	maxSamsungRedirectURILength  = 2 << 10
	samsungHTTPTimeout           = 5 * time.Second
	samsungTokenFutureSkew       = 2 * time.Minute
)

type SamsungOIDCTokenClientConfig struct {
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
}

type SamsungOIDCTokenClient struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewSamsungOIDCTokenClient(
	config SamsungOIDCTokenClientConfig,
) (*SamsungOIDCTokenClient, error) {
	if !validSamsungClientID(config.ClientID) {
		return nil, errors.New("Samsung OIDC client ID is invalid")
	}
	if config.ClientSecret == "" ||
		len(config.ClientSecret) > maxSamsungClientSecretLength ||
		strings.ContainsAny(config.ClientSecret, "\r\n") {
		return nil, errors.New("Samsung OIDC client secret is invalid")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: samsungHTTPTimeout}
	}
	return &SamsungOIDCTokenClient{
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		httpClient:   config.HTTPClient,
	}, nil
}

func (c *SamsungOIDCTokenClient) Exchange(
	ctx context.Context,
	code string,
	redirectURI string,
	codeVerifier string,
) (string, error) {
	if code == "" ||
		len(code) > maxSamsungAuthorizationCode ||
		strings.ContainsAny(code, "\r\n") ||
		!validSamsungRedirectURI(redirectURI) ||
		!validSamsungOpaqueValue(codeVerifier) {
		return "", ErrLoginRequestInvalid
	}
	form := url.Values{
		"grant_type":    []string{"authorization_code"},
		"code":          []string{code},
		"redirect_uri":  []string{redirectURI},
		"code_verifier": []string{codeVerifier},
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		samsungTokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("create Samsung token request: %w", err)
	}
	request.SetBasicAuth(c.clientID, c.clientSecret)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", &IdentityProviderUnavailableError{Cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		if response.StatusCode == http.StatusBadRequest {
			return "", ErrIdentityTokenInvalid
		}
		return "", &IdentityProviderUnavailableError{Cause: fmt.Errorf(
			"Samsung token endpoint returned HTTP %d",
			response.StatusCode,
		)}
	}
	body, err := io.ReadAll(
		io.LimitReader(response.Body, maxSamsungTokenResponseBytes+1),
	)
	if err != nil {
		return "", &IdentityProviderUnavailableError{Cause: fmt.Errorf(
			"read Samsung token response: %w",
			err,
		)}
	}
	if len(body) > maxSamsungTokenResponseBytes {
		return "", &IdentityProviderUnavailableError{
			Cause: errors.New("Samsung token response is too large"),
		}
	}
	var payload struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil ||
		payload.IDToken == "" ||
		len(payload.IDToken) > maxSamsungIDTokenLength {
		return "", &IdentityProviderUnavailableError{
			Cause: errors.New("Samsung token response is invalid"),
		}
	}
	return payload.IDToken, nil
}

type SamsungVerifierConfig struct {
	Keys     RSASigningKeyProvider
	ClientID string
	Now      func() time.Time
}

type SamsungVerifier struct {
	keys     RSASigningKeyProvider
	clientID string
	now      func() time.Time
}

type samsungIdentityClaims struct {
	Nonce           string `json:"nonce"`
	AuthorizedParty string `json:"azp,omitempty"`
	jwt.RegisteredClaims
}

func NewSamsungVerifier(
	config SamsungVerifierConfig,
) (*SamsungVerifier, error) {
	if config.Keys == nil {
		return nil, errors.New("Samsung signing-key provider is required")
	}
	if !validSamsungClientID(config.ClientID) {
		return nil, errors.New("Samsung OIDC client ID is invalid")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &SamsungVerifier{
		keys:     config.Keys,
		clientID: config.ClientID,
		now:      config.Now,
	}, nil
}

func (v *SamsungVerifier) Verify(
	ctx context.Context,
	serialized string,
	expectedNonce string,
) (ExternalIdentity, error) {
	if serialized == "" ||
		len(serialized) > maxSamsungIDTokenLength ||
		!validSamsungOpaqueValue(expectedNonce) {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	now := v.now().UTC()
	claims := &samsungIdentityClaims{}
	token, err := jwt.ParseWithClaims(
		serialized,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodRS256 {
				return nil, ErrIdentityTokenInvalid
			}
			keyID, ok := token.Header["kid"].(string)
			if !ok || keyID == "" || len(keyID) > maxAppleKeyIDLength {
				return nil, ErrIdentityTokenInvalid
			}
			return v.keys.SigningKey(ctx, keyID)
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithStrictDecoding(),
	)
	if err != nil {
		var fetchError *appleSigningKeyFetchError
		if errors.As(err, &fetchError) {
			return ExternalIdentity{}, &IdentityProviderUnavailableError{
				Cause: fetchError,
			}
		}
		return ExternalIdentity{}, fmt.Errorf("%w: %v", ErrIdentityTokenInvalid, err)
	}
	if token == nil || !token.Valid ||
		claims.Issuer != samsungIssuer ||
		claims.ExpiresAt == nil ||
		!now.Before(claims.ExpiresAt.Time) ||
		claims.IssuedAt == nil ||
		claims.IssuedAt.Time.After(now.Add(samsungTokenFutureSkew)) ||
		claims.Nonce != expectedNonce ||
		!samsungAudienceAllowed(
			claims.Audience,
			claims.AuthorizedParty,
			v.clientID,
		) {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	subject := claims.Subject
	if subject == "" ||
		len(subject) > maxSamsungSubjectLength ||
		!utf8.ValidString(subject) ||
		strings.TrimSpace(subject) != subject {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	return ExternalIdentity{
		Provider: "samsung",
		Subject:  subject,
	}, nil
}

func samsungAudienceAllowed(
	audiences jwt.ClaimStrings,
	authorizedParty string,
	clientID string,
) bool {
	found := false
	for _, audience := range audiences {
		if audience == clientID {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if len(audiences) > 1 {
		return authorizedParty == clientID
	}
	return authorizedParty == "" || authorizedParty == clientID
}

func validSamsungClientID(value string) bool {
	return value != "" &&
		len(value) <= maxSamsungClientIDLength &&
		!strings.ContainsAny(value, ":\r\n")
}

func validSamsungRedirectURI(value string) bool {
	if value == "" ||
		len(value) > maxSamsungRedirectURILength ||
		strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil &&
		parsed.IsAbs() &&
		strings.EqualFold(parsed.Scheme, "https") &&
		parsed.Host != "" &&
		parsed.User == nil &&
		parsed.Fragment == ""
}
