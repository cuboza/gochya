package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleIssuer                          = "https://appleid.apple.com"
	appleJWKURL                          = appleIssuer + "/auth/keys"
	maxAppleIdentityTokenLength          = 16 << 10
	maxAppleSubjectLength                = 255
	maxAppleKeyIDLength                  = 128
	maxAppleJWKResponseBytes       int64 = 64 << 10
	defaultAppleJWKCacheTTL              = 5 * time.Minute
	maxAppleJWKCacheTTL                  = 24 * time.Hour
	defaultAppleJWKRefreshInterval       = 30 * time.Second
	appleJWKTimeout                      = 5 * time.Second
	appleTokenFutureSkew                 = 2 * time.Minute
)

var errAppleSigningKeyNotFound = errors.New("Apple signing key was not found")

type appleSigningKeyFetchError struct {
	provider string
	cause    error
}

func (e *appleSigningKeyFetchError) Error() string {
	provider := e.provider
	if provider == "" {
		provider = "identity provider"
	}
	return "fetch " + provider + " signing keys: " + e.cause.Error()
}

func (e *appleSigningKeyFetchError) Unwrap() error {
	return e.cause
}

type RSASigningKeyProvider interface {
	SigningKey(context.Context, string) (*rsa.PublicKey, error)
}

type AppleSigningKeyProvider = RSASigningKeyProvider

type HTTPRSAKeySetConfig struct {
	HTTPClient         *http.Client
	Now                func() time.Time
	DefaultCacheTTL    time.Duration
	MinRefreshInterval time.Duration
}

type AppleHTTPKeySetConfig = HTTPRSAKeySetConfig

type HTTPRSAKeySet struct {
	client             *http.Client
	now                func() time.Time
	defaultCacheTTL    time.Duration
	minRefreshInterval time.Duration
	endpoint           string
	provider           string

	mutex         sync.Mutex
	keys          map[string]*rsa.PublicKey
	expiresAt     time.Time
	nextRefreshAt time.Time
	initialized   bool
}

type AppleHTTPKeySet = HTTPRSAKeySet
type SamsungHTTPKeySet = HTTPRSAKeySet

func NewAppleHTTPKeySet(
	config AppleHTTPKeySetConfig,
) (*AppleHTTPKeySet, error) {
	return newHTTPRSAKeySet(config, "Apple", appleJWKURL)
}

func NewSamsungHTTPKeySet(
	config HTTPRSAKeySetConfig,
) (*SamsungHTTPKeySet, error) {
	return newHTTPRSAKeySet(config, "Samsung", samsungJWKURL)
}

func newHTTPRSAKeySet(
	config HTTPRSAKeySetConfig,
	provider string,
	endpoint string,
) (*HTTPRSAKeySet, error) {
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: appleJWKTimeout}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.DefaultCacheTTL == 0 {
		config.DefaultCacheTTL = defaultAppleJWKCacheTTL
	}
	if config.DefaultCacheTTL < time.Second ||
		config.DefaultCacheTTL > maxAppleJWKCacheTTL {
		return nil, errors.New(
			"OIDC JWK default cache TTL must be between one second and 24 hours",
		)
	}
	if config.MinRefreshInterval == 0 {
		config.MinRefreshInterval = defaultAppleJWKRefreshInterval
	}
	if config.MinRefreshInterval < 0 ||
		config.MinRefreshInterval > config.DefaultCacheTTL {
		return nil, errors.New("OIDC JWK refresh interval is invalid")
	}
	return &HTTPRSAKeySet{
		client:             config.HTTPClient,
		now:                config.Now,
		defaultCacheTTL:    config.DefaultCacheTTL,
		minRefreshInterval: config.MinRefreshInterval,
		endpoint:           endpoint,
		provider:           provider,
		keys:               make(map[string]*rsa.PublicKey),
	}, nil
}

func (s *HTTPRSAKeySet) SigningKey(
	ctx context.Context,
	keyID string,
) (*rsa.PublicKey, error) {
	if s == nil || s.client == nil || s.now == nil {
		return nil, &appleSigningKeyFetchError{
			provider: "identity provider",
			cause:    errors.New("signing-key set is not configured"),
		}
	}
	if keyID == "" || len(keyID) > maxAppleKeyIDLength {
		return nil, errAppleSigningKeyNotFound
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, &appleSigningKeyFetchError{
			provider: s.provider,
			cause:    err,
		}
	}
	now := s.now().UTC()
	cacheFresh := s.initialized && now.Before(s.expiresAt)
	if cacheFresh {
		if key := s.keys[keyID]; key != nil {
			return cloneRSAPublicKey(key), nil
		}
		if now.Before(s.nextRefreshAt) {
			return nil, errAppleSigningKeyNotFound
		}
	}
	keys, cacheTTL, err := s.fetch(ctx)
	if err != nil {
		return nil, &appleSigningKeyFetchError{
			provider: s.provider,
			cause:    err,
		}
	}
	s.keys = keys
	s.expiresAt = now.Add(cacheTTL)
	s.nextRefreshAt = now.Add(s.minRefreshInterval)
	s.initialized = true
	key := s.keys[keyID]
	if key == nil {
		return nil, errAppleSigningKeyNotFound
	}
	return cloneRSAPublicKey(key), nil
}

func (s *HTTPRSAKeySet) fetch(
	ctx context.Context,
) (map[string]*rsa.PublicKey, time.Duration, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		s.endpoint,
		nil,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("create JWK request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, 0, fmt.Errorf(
			"JWK endpoint returned HTTP %d",
			response.StatusCode,
		)
	}
	body, err := io.ReadAll(
		io.LimitReader(response.Body, maxAppleJWKResponseBytes+1),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("read JWK response: %w", err)
	}
	if int64(len(body)) > maxAppleJWKResponseBytes {
		return nil, 0, errors.New("JWK response is too large")
	}
	keys, err := decodeAppleJWKSet(body)
	if err != nil {
		return nil, 0, err
	}
	return keys, appleJWKCacheTTL(
		response.Header.Get("Cache-Control"),
		s.defaultCacheTTL,
	), nil
}

type appleJWKSet struct {
	Keys []appleJWK `json:"keys"`
}

type appleJWK struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func decodeAppleJWKSet(body []byte) (map[string]*rsa.PublicKey, error) {
	var document appleJWKSet
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode JWK response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("JWK response contains trailing JSON")
	}
	if len(document.Keys) == 0 || len(document.Keys) > 16 {
		return nil, errors.New("JWK response contains an invalid key count")
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, serialized := range document.Keys {
		if serialized.KeyType != "RSA" ||
			serialized.Use != "sig" ||
			serialized.Algorithm != jwt.SigningMethodRS256.Alg() {
			continue
		}
		if serialized.KeyID == "" ||
			len(serialized.KeyID) > maxAppleKeyIDLength {
			return nil, errors.New("JWK response contains an invalid key ID")
		}
		if _, duplicate := keys[serialized.KeyID]; duplicate {
			return nil, errors.New("JWK response contains a duplicate key ID")
		}
		key, err := decodeAppleRSAKey(serialized)
		if err != nil {
			return nil, fmt.Errorf(
				"decode Apple JWK %q: %w",
				serialized.KeyID,
				err,
			)
		}
		keys[serialized.KeyID] = key
	}
	if len(keys) == 0 {
		return nil, errors.New("JWK response contains no supported signing keys")
	}
	return keys, nil
}

func decodeAppleRSAKey(serialized appleJWK) (*rsa.PublicKey, error) {
	modulus, err := decodeCanonicalBase64URL(serialized.Modulus)
	if err != nil || len(modulus) == 0 {
		return nil, errors.New("RSA modulus is invalid")
	}
	exponentBytes, err := decodeCanonicalBase64URL(serialized.Exponent)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, errors.New("RSA exponent is invalid")
	}
	exponent := uint64(0)
	for _, value := range exponentBytes {
		exponent = exponent<<8 | uint64(value)
	}
	if exponent < 3 || exponent > uint64(^uint(0)>>1) || exponent%2 == 0 {
		return nil, errors.New("RSA exponent is invalid")
	}
	key := &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulus),
		E: int(exponent),
	}
	if key.N.BitLen() < 2048 || key.N.BitLen() > 8192 {
		return nil, errors.New("RSA modulus size is outside the accepted range")
	}
	return key, nil
}

func decodeCanonicalBase64URL(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil ||
		base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("value is not canonical unpadded base64url")
	}
	return decoded, nil
}

func appleJWKCacheTTL(cacheControl string, fallback time.Duration) time.Duration {
	for _, directive := range strings.Split(cacheControl, ",") {
		name, value, found := strings.Cut(strings.TrimSpace(directive), "=")
		if !found || !strings.EqualFold(name, "max-age") {
			continue
		}
		seconds, err := strconv.ParseInt(strings.Trim(value, `"`), 10, 64)
		if err != nil || seconds <= 0 {
			break
		}
		ttl := time.Duration(seconds) * time.Second
		if ttl > maxAppleJWKCacheTTL {
			return maxAppleJWKCacheTTL
		}
		return ttl
	}
	return fallback
}

func cloneRSAPublicKey(key *rsa.PublicKey) *rsa.PublicKey {
	return &rsa.PublicKey{N: new(big.Int).Set(key.N), E: key.E}
}

type AppleVerifierConfig struct {
	Keys      AppleSigningKeyProvider
	Audiences map[string]struct{}
	Now       func() time.Time
}

type AppleVerifier struct {
	keys      AppleSigningKeyProvider
	audiences []string
	now       func() time.Time
}

type appleIdentityClaims struct {
	Nonce string `json:"nonce"`
	jwt.RegisteredClaims
}

func NewAppleVerifier(config AppleVerifierConfig) (*AppleVerifier, error) {
	if config.Keys == nil {
		return nil, errors.New("Apple signing-key provider is required")
	}
	if len(config.Audiences) == 0 {
		return nil, errors.New("Apple client-ID allowlist is required")
	}
	audiences := make([]string, 0, len(config.Audiences))
	for audience := range config.Audiences {
		audience = strings.TrimSpace(audience)
		if audience == "" || len(audience) > 512 {
			return nil, errors.New("Apple client ID is invalid")
		}
		audiences = append(audiences, audience)
	}
	slices.Sort(audiences)
	if config.Now == nil {
		config.Now = time.Now
	}
	return &AppleVerifier{
		keys:      config.Keys,
		audiences: audiences,
		now:       config.Now,
	}, nil
}

func (v *AppleVerifier) Verify(
	ctx context.Context,
	serialized string,
	expectedNonce string,
) (ExternalIdentity, error) {
	if serialized == "" ||
		len(serialized) > maxAppleIdentityTokenLength ||
		!validAppleLoginNonce(expectedNonce) {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	now := v.now().UTC()
	claims := &appleIdentityClaims{}
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
		claims.Issuer != appleIssuer ||
		claims.ExpiresAt == nil ||
		!now.Before(claims.ExpiresAt.Time) ||
		claims.IssuedAt == nil ||
		claims.IssuedAt.Time.After(now.Add(appleTokenFutureSkew)) ||
		claims.Nonce != expectedNonce ||
		!appleAudienceAllowed(claims.Audience, v.audiences) {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	subject := claims.Subject
	if subject == "" ||
		len(subject) > maxAppleSubjectLength ||
		!utf8.ValidString(subject) ||
		strings.TrimSpace(subject) != subject {
		return ExternalIdentity{}, ErrIdentityTokenInvalid
	}
	return ExternalIdentity{
		Provider: "apple",
		Subject:  subject,
	}, nil
}

func appleAudienceAllowed(
	tokenAudiences jwt.ClaimStrings,
	allowed []string,
) bool {
	for _, tokenAudience := range tokenAudiences {
		if slices.Contains(allowed, tokenAudience) {
			return true
		}
	}
	return false
}
