package dojo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultPlayIntegrityBaseURL = "https://playintegrity.googleapis.com"

type PlayIntegrityAccessTokenSource interface {
	AccessToken(context.Context) (string, error)
}

type PlayIntegrityAccessTokenSourceFunc func(context.Context) (string, error)

func (fn PlayIntegrityAccessTokenSourceFunc) AccessToken(ctx context.Context) (string, error) {
	return fn(ctx)
}

type HTTPPlayIntegrityDecoderConfig struct {
	AccessTokens      PlayIntegrityAccessTokenSource
	HTTPClient        *http.Client
	BaseURL           string
	AllowInsecureHTTP bool
}

type HTTPPlayIntegrityDecoder struct {
	accessTokens PlayIntegrityAccessTokenSource
	httpClient   *http.Client
	baseURL      string
}

var _ PlayIntegrityDecoder = (*HTTPPlayIntegrityDecoder)(nil)

func NewHTTPPlayIntegrityDecoder(
	config HTTPPlayIntegrityDecoderConfig,
) (*HTTPPlayIntegrityDecoder, error) {
	if config.AccessTokens == nil {
		return nil, errors.New("Play Integrity access-token source is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultPlayIntegrityBaseURL
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Play Integrity base URL is invalid")
	}
	if parsed.Scheme != "https" && !config.AllowInsecureHTTP {
		return nil, errors.New("Play Integrity base URL must use HTTPS")
	}
	return &HTTPPlayIntegrityDecoder{
		accessTokens: config.AccessTokens,
		httpClient:   config.HTTPClient,
		baseURL:      strings.TrimRight(config.BaseURL, "/"),
	}, nil
}

func (d *HTTPPlayIntegrityDecoder) Decode(
	ctx context.Context,
	packageName string,
	integrityToken string,
) (PlayIntegrityPayload, error) {
	accessToken, err := d.accessTokens.AccessToken(ctx)
	if err != nil {
		return PlayIntegrityPayload{}, &AttestationUnavailableError{Cause: err}
	}
	if accessToken == "" || strings.ContainsAny(accessToken, " \t\r\n") {
		return PlayIntegrityPayload{}, &AttestationUnavailableError{
			Cause: errors.New("Play Integrity access token is invalid"),
		}
	}
	requestBody, err := json.Marshal(struct {
		IntegrityToken string `json:"integrity_token"`
	}{IntegrityToken: integrityToken})
	if err != nil {
		return PlayIntegrityPayload{}, fmt.Errorf("encode Play Integrity request: %w", err)
	}
	endpoint := d.baseURL + "/v1/" + url.PathEscape(packageName) + ":decodeIntegrityToken"
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return PlayIntegrityPayload{}, fmt.Errorf("create Play Integrity request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := d.httpClient.Do(request)
	if err != nil {
		return PlayIntegrityPayload{}, &AttestationUnavailableError{Cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		statusErr := fmt.Errorf("Play Integrity decode returned HTTP %d", response.StatusCode)
		if response.StatusCode == http.StatusUnauthorized ||
			response.StatusCode == http.StatusForbidden ||
			response.StatusCode == http.StatusTooManyRequests ||
			response.StatusCode >= http.StatusInternalServerError {
			return PlayIntegrityPayload{}, &AttestationUnavailableError{Cause: statusErr}
		}
		return PlayIntegrityPayload{}, statusErr
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil {
		return PlayIntegrityPayload{}, &AttestationUnavailableError{
			Cause: fmt.Errorf("read Play Integrity response: %w", err),
		}
	}
	if len(responseBody) > 64<<10 {
		return PlayIntegrityPayload{}, &AttestationUnavailableError{
			Cause: errors.New("Play Integrity response is too large"),
		}
	}
	var decoded struct {
		TokenPayloadExternal PlayIntegrityPayload `json:"tokenPayloadExternal"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return PlayIntegrityPayload{}, &AttestationUnavailableError{
			Cause: fmt.Errorf("decode Play Integrity response: %w", err),
		}
	}
	return decoded.TokenPayloadExternal, nil
}
