package dojo

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	playIntegrityStandardProvider = "play_integrity_standard"
	defaultPlayResultCacheTTL     = 2 * time.Minute
	maxPlayResultCacheEntries     = 4096
)

type PlayIntegrityDecoder interface {
	Decode(context.Context, string, string) (PlayIntegrityPayload, error)
}

type PlayIntegrityPayload struct {
	RequestDetails struct {
		RequestPackageName string `json:"requestPackageName"`
		RequestHash        string `json:"requestHash"`
		TimestampMillis    string `json:"timestampMillis"`
	} `json:"requestDetails"`
	AppIntegrity struct {
		AppRecognitionVerdict   string   `json:"appRecognitionVerdict"`
		PackageName             string   `json:"packageName"`
		CertificateSHA256Digest []string `json:"certificateSha256Digest"`
		VersionCode             string   `json:"versionCode"`
	} `json:"appIntegrity"`
	AccountDetails struct {
		AppLicensingVerdict string `json:"appLicensingVerdict"`
	} `json:"accountDetails"`
	DeviceIntegrity struct {
		DeviceRecognitionVerdict []string `json:"deviceRecognitionVerdict"`
	} `json:"deviceIntegrity"`
	TestingDetails struct {
		IsTestingResponse bool `json:"isTestingResponse"`
	} `json:"testingDetails"`
}

type PlayIntegrityVerifierConfig struct {
	Decoder                  PlayIntegrityDecoder
	PackageName              string
	CertificateSHA256Digests map[string]struct{}
	RequiredDeviceVerdicts   []string
	AllowUnlicensed          bool
	AllowTestingResponses    bool
	MaxVerdictAge            time.Duration
	FutureSkew               time.Duration
	ResultCacheTTL           time.Duration
	Now                      func() time.Time
}

type PlayIntegrityVerifier struct {
	decoder                PlayIntegrityDecoder
	packageName            string
	certificateDigests     map[string]struct{}
	requiredDeviceVerdicts []string
	allowUnlicensed        bool
	allowTestingResponses  bool
	maxVerdictAge          time.Duration
	futureSkew             time.Duration
	resultCacheTTL         time.Duration
	now                    func() time.Time
	mu                     sync.Mutex
	inflight               map[[32]byte]*playIntegrityFlight
	results                map[[32]byte]playIntegrityResult
}

var _ AttestationVerifier = (*PlayIntegrityVerifier)(nil)

type playIntegrityFlight struct {
	done chan struct{}
	err  error
}

type playIntegrityResult struct {
	err       error
	expiresAt time.Time
}

func NewPlayIntegrityVerifier(
	config PlayIntegrityVerifierConfig,
) (*PlayIntegrityVerifier, error) {
	if config.Decoder == nil {
		return nil, errors.New("Play Integrity decoder is required")
	}
	if strings.TrimSpace(config.PackageName) == "" {
		return nil, errors.New("Play Integrity package name is required")
	}
	if len(config.CertificateSHA256Digests) == 0 {
		return nil, errors.New("Play Integrity certificate allowlist is required")
	}
	certificateDigests := make(map[string]struct{}, len(config.CertificateSHA256Digests))
	for digest := range config.CertificateSHA256Digests {
		if strings.TrimSpace(digest) == "" || len(digest) > 128 {
			return nil, errors.New("Play Integrity certificate digest is invalid")
		}
		certificateDigests[digest] = struct{}{}
	}
	if len(config.RequiredDeviceVerdicts) == 0 {
		config.RequiredDeviceVerdicts = []string{"MEETS_DEVICE_INTEGRITY"}
	}
	requiredDeviceVerdicts := make([]string, len(config.RequiredDeviceVerdicts))
	for index, verdict := range config.RequiredDeviceVerdicts {
		if strings.TrimSpace(verdict) == "" {
			return nil, errors.New("required Play Integrity device verdict is invalid")
		}
		requiredDeviceVerdicts[index] = verdict
	}
	if config.MaxVerdictAge == 0 {
		config.MaxVerdictAge = 2 * time.Minute
	}
	if config.FutureSkew == 0 {
		config.FutureSkew = 30 * time.Second
	}
	if config.MaxVerdictAge <= 0 || config.MaxVerdictAge > 10*time.Minute {
		return nil, errors.New("Play Integrity verdict age must be between zero and ten minutes")
	}
	if config.FutureSkew < 0 || config.FutureSkew > 2*time.Minute {
		return nil, errors.New("Play Integrity future skew must be between zero and two minutes")
	}
	if config.ResultCacheTTL == 0 {
		config.ResultCacheTTL = defaultPlayResultCacheTTL
	}
	if config.ResultCacheTTL <= 0 || config.ResultCacheTTL > 10*time.Minute {
		return nil, errors.New("Play Integrity result cache TTL must be between zero and ten minutes")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &PlayIntegrityVerifier{
		decoder:                config.Decoder,
		packageName:            config.PackageName,
		certificateDigests:     certificateDigests,
		requiredDeviceVerdicts: requiredDeviceVerdicts,
		allowUnlicensed:        config.AllowUnlicensed,
		allowTestingResponses:  config.AllowTestingResponses,
		maxVerdictAge:          config.MaxVerdictAge,
		futureSkew:             config.FutureSkew,
		resultCacheTTL:         config.ResultCacheTTL,
		now:                    config.Now,
		inflight:               make(map[[32]byte]*playIntegrityFlight),
		results:                make(map[[32]byte]playIntegrityResult),
	}, nil
}

func (v *PlayIntegrityVerifier) Verify(
	ctx context.Context,
	input AttestationInput,
) error {
	if input.Evidence.Provider != playIntegrityStandardProvider {
		return errors.New("attestation provider is not Play Integrity Standard")
	}
	if input.Evidence.Token == "" || len(input.Evidence.Token) > 16<<10 {
		return errors.New("Play Integrity token is missing or too large")
	}
	if input.RequestHash == "" {
		return errors.New("Play Integrity request hash is missing")
	}
	key := playIntegrityCacheKey(input.Evidence.Token, input.RequestHash)
	now := v.now()
	v.mu.Lock()
	v.pruneResultCache(now)
	if result, ok := v.results[key]; ok {
		v.mu.Unlock()
		return result.err
	}
	if flight, ok := v.inflight[key]; ok {
		v.mu.Unlock()
		select {
		case <-flight.done:
			return flight.err
		case <-ctx.Done():
			return &AttestationUnavailableError{Cause: ctx.Err()}
		}
	}
	flight := &playIntegrityFlight{done: make(chan struct{})}
	v.inflight[key] = flight
	v.mu.Unlock()

	err := v.verifyRemote(ctx, input, now)
	v.mu.Lock()
	delete(v.inflight, key)
	flight.err = err
	var unavailable *AttestationUnavailableError
	if !errors.As(err, &unavailable) && len(v.results) < maxPlayResultCacheEntries {
		v.results[key] = playIntegrityResult{
			err:       err,
			expiresAt: now.Add(v.resultCacheTTL),
		}
	}
	close(flight.done)
	v.mu.Unlock()
	return err
}

func (v *PlayIntegrityVerifier) verifyRemote(
	ctx context.Context,
	input AttestationInput,
	now time.Time,
) error {
	payload, err := v.decoder.Decode(ctx, v.packageName, input.Evidence.Token)
	if err != nil {
		var unavailable *AttestationUnavailableError
		if errors.As(err, &unavailable) {
			return err
		}
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return &AttestationUnavailableError{Cause: err}
		}
		return err
	}
	if payload.RequestDetails.RequestPackageName != v.packageName ||
		payload.RequestDetails.RequestHash != input.RequestHash {
		return errors.New("Play Integrity request binding is invalid")
	}
	timestampMillis, err := strconv.ParseInt(
		payload.RequestDetails.TimestampMillis,
		10,
		64,
	)
	if err != nil {
		return errors.New("Play Integrity timestamp is invalid")
	}
	verdictTime := time.UnixMilli(timestampMillis)
	if verdictTime.Before(now.Add(-v.maxVerdictAge)) ||
		verdictTime.After(now.Add(v.futureSkew)) {
		return errors.New("Play Integrity verdict is stale")
	}
	if payload.AppIntegrity.AppRecognitionVerdict != "PLAY_RECOGNIZED" ||
		payload.AppIntegrity.PackageName != v.packageName ||
		payload.AppIntegrity.VersionCode != input.AppBuild {
		return errors.New("Play Integrity app verdict is invalid")
	}
	if !matchesCertificate(
		payload.AppIntegrity.CertificateSHA256Digest,
		v.certificateDigests,
	) {
		return errors.New("Play Integrity signing certificate is not allowed")
	}
	if !v.allowUnlicensed &&
		payload.AccountDetails.AppLicensingVerdict != "LICENSED" {
		return errors.New("Play Integrity app license is invalid")
	}
	for _, required := range v.requiredDeviceVerdicts {
		if !containsString(payload.DeviceIntegrity.DeviceRecognitionVerdict, required) {
			return fmt.Errorf("Play Integrity device verdict %q is missing", required)
		}
	}
	if payload.TestingDetails.IsTestingResponse && !v.allowTestingResponses {
		return errors.New("Play Integrity testing response is not allowed")
	}
	return nil
}

func (v *PlayIntegrityVerifier) pruneResultCache(now time.Time) {
	for key, result := range v.results {
		if !now.Before(result.expiresAt) {
			delete(v.results, key)
		}
	}
}

func playIntegrityCacheKey(token, requestHash string) [32]byte {
	digest := sha256.New()
	_, _ = digest.Write([]byte(token))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(requestHash))
	var key [32]byte
	copy(key[:], digest.Sum(nil))
	return key
}

func matchesCertificate(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
