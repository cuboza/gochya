package dojo

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	defaultNonceTTL = 5 * time.Minute
	maxStringLength = 128
)

type ServiceConfig struct {
	Store                     Store
	Core                      corebridge.Engine
	Attestation               AttestationVerifier
	AllowedAppBuilds          map[string]struct{}
	AllowedClassifierVersions map[string]struct{}
	Now                       func() time.Time
	Random                    io.Reader
	NonceTTL                  time.Duration
}

type Service struct {
	store                     Store
	core                      corebridge.Engine
	attestation               AttestationVerifier
	allowedAppBuilds          map[string]struct{}
	allowedClassifierVersions map[string]struct{}
	now                       func() time.Time
	random                    io.Reader
	nonceTTL                  time.Duration
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Core == nil || config.Attestation == nil {
		return nil, errors.New("store, core and attestation verifier are required")
	}
	if len(config.AllowedAppBuilds) == 0 || len(config.AllowedClassifierVersions) == 0 {
		return nil, errors.New("app build and classifier allowlists are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	if config.NonceTTL == 0 {
		config.NonceTTL = defaultNonceTTL
	}
	if config.NonceTTL <= 0 {
		return nil, errors.New("nonce TTL must be positive")
	}
	return &Service{
		store:                     config.Store,
		core:                      config.Core,
		attestation:               config.Attestation,
		allowedAppBuilds:          cloneSet(config.AllowedAppBuilds),
		allowedClassifierVersions: cloneSet(config.AllowedClassifierVersions),
		now:                       config.Now,
		random:                    config.Random,
		nonceTTL:                  config.NonceTTL,
	}, nil
}

func (s *Service) Preflight(
	ctx context.Context,
	playerID string,
	request PreflightRequest,
) (PreflightResponse, error) {
	if err := validateIdentity(playerID, request.DeviceID); err != nil {
		return PreflightResponse{}, err
	}
	if _, ok := s.allowedAppBuilds[request.AppBuild]; !ok {
		return PreflightResponse{}, apiError(
			"unsupported_build",
			"app build is not allowed",
			http.StatusBadRequest,
		)
	}
	device, err := s.store.Device(ctx, playerID, request.DeviceID)
	if err != nil || !device.Enabled || len(device.PublicKey) != ed25519.PublicKeySize {
		return PreflightResponse{}, apiError(
			"device_invalid",
			"registered device is required",
			http.StatusForbidden,
		)
	}
	nonce, err := randomToken(s.random, 32)
	if err != nil {
		return PreflightResponse{}, asAPIError(fmt.Errorf("generate nonce: %w", err))
	}
	challenge, err := randomToken(s.random, 32)
	if err != nil {
		return PreflightResponse{}, asAPIError(fmt.Errorf("generate challenge: %w", err))
	}
	now := s.now().UTC()
	record := NonceRecord{
		Value:                 nonce,
		Challenge:             challenge,
		PlayerID:              playerID,
		DeviceID:              request.DeviceID,
		AppBuild:              request.AppBuild,
		EvidenceSchemaVersion: EvidenceSchemaV1,
		IssuedAt:              now,
		ExpiresAt:             now.Add(s.nonceTTL),
	}
	if err := s.store.PutNonce(ctx, record); err != nil {
		return PreflightResponse{}, asAPIError(fmt.Errorf("store nonce: %w", err))
	}
	return PreflightResponse{
		Nonce:                 record.Value,
		Challenge:             record.Challenge,
		EvidenceSchemaVersion: record.EvidenceSchemaVersion,
		ExpiresAt:             record.ExpiresAt,
	}, nil
}

func (s *Service) Submit(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request SubmitRequest,
) (SubmitResponse, error) {
	if err := validateUUID(idempotencyKey); err != nil {
		return SubmitResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	requestHash, err := hashJSON(request)
	if err != nil {
		return SubmitResponse{}, asAPIError(fmt.Errorf("hash request: %w", err))
	}
	if response, ok, err := s.idempotentSubmit(
		ctx,
		playerID,
		idempotencyKey,
		requestHash,
	); err != nil || ok {
		return response, err
	}
	if err := s.validateSubmitEnvelope(playerID, request); err != nil {
		return SubmitResponse{}, err
	}

	now := s.now().UTC()
	nonce, err := s.store.Nonce(ctx, request.Nonce)
	if err != nil ||
		nonce.PlayerID != playerID ||
		nonce.DeviceID != request.DeviceID ||
		nonce.AppBuild != request.AppBuild ||
		nonce.EvidenceSchemaVersion != request.EvidenceSchemaVersion {
		return SubmitResponse{}, nonceInvalidError()
	}
	if !now.Before(nonce.ExpiresAt) {
		return SubmitResponse{}, nonceInvalidError()
	}
	if nonce.UsedAt != nil {
		response, ok, err := s.idempotentSubmit(
			ctx,
			playerID,
			idempotencyKey,
			requestHash,
		)
		if err != nil || ok {
			return response, err
		}
		return SubmitResponse{}, replayError()
	}

	device, err := s.store.Device(ctx, playerID, request.DeviceID)
	if err != nil || !device.Enabled || len(device.PublicKey) != ed25519.PublicKeySize {
		return SubmitResponse{}, apiError(
			"device_invalid",
			"registered device is required",
			http.StatusForbidden,
		)
	}
	canonical, err := CanonicalPayload(request)
	if err != nil {
		return SubmitResponse{}, asAPIError(fmt.Errorf("canonical payload: %w", err))
	}
	signature, err := base64.RawURLEncoding.DecodeString(request.PayloadSignature)
	if err != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(device.PublicKey, canonical, signature) {
		return SubmitResponse{}, apiError(
			"signature_invalid",
			"payload signature is invalid",
			http.StatusUnauthorized,
		)
	}
	if err := s.attestation.Verify(ctx, AttestationInput{
		PlayerID:    playerID,
		DeviceID:    request.DeviceID,
		AppBuild:    request.AppBuild,
		Nonce:       request.Nonce,
		Challenge:   nonce.Challenge,
		RequestHash: playIntegrityRequestHash(nonce.Challenge, canonical),
		Evidence:    request.Attestation,
	}); err != nil {
		var unavailable *AttestationUnavailableError
		if errors.As(err, &unavailable) {
			return SubmitResponse{}, &Error{
				Code:       "attestation_unavailable",
				Message:    "device attestation is temporarily unavailable",
				HTTPStatus: http.StatusServiceUnavailable,
				Cause:      err,
			}
		}
		return SubmitResponse{}, &Error{
			Code:       "attestation_invalid",
			Message:    "device attestation is invalid",
			HTTPStatus: http.StatusUnauthorized,
			Cause:      err,
		}
	}
	verdict, heart, err := validateEvidence(request, nonce, now)
	if err != nil {
		return SubmitResponse{}, err
	}
	heartVerdict, err := s.core.ValidateHeart(ctx, heart)
	if err != nil {
		return SubmitResponse{}, coreError(err)
	}
	if !heartVerdict.Passed {
		return SubmitResponse{}, &Error{
			Code:       "heart_rejected",
			Message:    "heart-rate evidence did not pass the activity gate",
			HTTPStatus: http.StatusUnprocessableEntity,
			Details:    map[string]any{"reason": heartVerdict.Reason},
		}
	}
	stats, err := s.core.DeriveTechnique(ctx, toCoreMetrics(request.Metrics), heart, 1)
	if err != nil {
		return SubmitResponse{}, coreError(err)
	}
	if stats.TechniqueType != request.Metrics.TechniqueType {
		return SubmitResponse{}, asAPIError(errors.New("core returned another technique type"))
	}
	element, err := s.store.ActiveElement(ctx, playerID)
	if err != nil {
		return SubmitResponse{}, asAPIError(fmt.Errorf("load active element: %w", err))
	}
	cardID, err := randomUUID(s.random)
	if err != nil {
		return SubmitResponse{}, asAPIError(fmt.Errorf("generate card ID: %w", err))
	}
	response := SubmitResponse{
		Card: TechniqueCard{
			ID:          cardID,
			OwnerID:     playerID,
			Type:        stats.TechniqueType,
			Element:     element,
			Rarity:      stats.Rarity,
			BaseDamage:  stats.BaseDamage,
			Speed:       stats.Speed,
			StaminaCost: stats.StaminaCost,
			CritChance:  stats.CritChance,
			Quality:     stats.Quality,
			CreatedAt:   now,
		},
		EvidenceVerdict: verdict,
	}
	replayHash, err := replayHash(request)
	if err != nil {
		return SubmitResponse{}, asAPIError(fmt.Errorf("replay hash: %w", err))
	}
	committed, err := s.store.CommitSubmit(ctx, CommitRequest{
		PlayerID:              playerID,
		DeviceID:              request.DeviceID,
		AppBuild:              request.AppBuild,
		ClassifierVersion:     request.ClassifierVersion,
		EvidenceSchemaVersion: request.EvidenceSchemaVersion,
		Nonce:                 request.Nonce,
		IdempotencyKey:        idempotencyKey,
		RequestHash:           requestHash,
		ReplayHash:            replayHash,
		Response:              response,
		Now:                   now,
	})
	if err != nil {
		return SubmitResponse{}, mapCommitError(err)
	}
	return committed, nil
}

func (s *Service) idempotentSubmit(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	requestHash [32]byte,
) (SubmitResponse, bool, error) {
	response, storedHash, ok, err := s.store.Idempotency(
		ctx,
		playerID,
		idempotencyKey,
	)
	if err != nil {
		return SubmitResponse{}, false, asAPIError(err)
	}
	if !ok {
		return SubmitResponse{}, false, nil
	}
	if storedHash != requestHash {
		return SubmitResponse{}, false, idempotencyConflictError()
	}
	return response, true, nil
}

type signedPayload struct {
	DeviceID              string         `json:"deviceId"`
	Nonce                 string         `json:"nonce"`
	EvidenceSchemaVersion uint16         `json:"evidenceSchemaVersion"`
	RecordedAtMS          int64          `json:"recordedAtMs"`
	Metrics               Metrics        `json:"metrics"`
	HeartEvidence         HeartEvidence  `json:"heartEvidence"`
	FeatureSummary        FeatureSummary `json:"featureSummary"`
	ClassifierVersion     string         `json:"classifierVersion"`
	AppBuild              string         `json:"appBuild"`
}

func CanonicalPayload(request SubmitRequest) ([]byte, error) {
	return json.Marshal(signedPayload{
		DeviceID:              request.DeviceID,
		Nonce:                 request.Nonce,
		EvidenceSchemaVersion: request.EvidenceSchemaVersion,
		RecordedAtMS:          request.RecordedAtMS,
		Metrics:               request.Metrics,
		HeartEvidence:         request.HeartEvidence,
		FeatureSummary:        request.FeatureSummary,
		ClassifierVersion:     request.ClassifierVersion,
		AppBuild:              request.AppBuild,
	})
}

func PlayIntegrityRequestHash(challenge string, request SubmitRequest) (string, error) {
	canonical, err := CanonicalPayload(request)
	if err != nil {
		return "", err
	}
	return playIntegrityRequestHash(challenge, canonical), nil
}

func playIntegrityRequestHash(challenge string, canonical []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("gochya-dojo-play-integrity-v1"))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(challenge))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(canonical)
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

func (s *Service) validateSubmitEnvelope(playerID string, request SubmitRequest) error {
	if err := validateIdentity(playerID, request.DeviceID); err != nil {
		return err
	}
	if request.Nonce == "" || len(request.Nonce) > maxStringLength {
		return validationError("nonce is required")
	}
	if request.EvidenceSchemaVersion != EvidenceSchemaV1 {
		return apiError(
			"unsupported_evidence_schema",
			"evidence schema version is not supported",
			http.StatusBadRequest,
		)
	}
	if _, ok := s.allowedAppBuilds[request.AppBuild]; !ok {
		return apiError("unsupported_build", "app build is not allowed", http.StatusBadRequest)
	}
	if _, ok := s.allowedClassifierVersions[request.ClassifierVersion]; !ok {
		return apiError(
			"unsupported_classifier",
			"classifier version is not allowed",
			http.StatusBadRequest,
		)
	}
	if request.Attestation.Provider == "" || request.Attestation.Token == "" ||
		len(request.Attestation.Provider) > 32 || len(request.Attestation.Token) > 8192 {
		return validationError("attestation evidence is required")
	}
	return nil
}

func validateEvidence(
	request SubmitRequest,
	nonce NonceRecord,
	now time.Time,
) (string, corebridge.HeartEvidence, error) {
	metrics := request.Metrics
	summary := request.FeatureSummary
	heart := request.HeartEvidence
	if !allFinite(
		metrics.PeakAccelMPS2,
		metrics.ExecTimeSeconds,
		metrics.Precision,
		metrics.RhythmScore,
		heart.Present,
		heart.ContactConfidence,
		summary.AccelPeakMPS2,
		summary.AccelRMSMPS2,
		summary.GyroPeakRadiansS,
		summary.GyroRMSRadiansS,
		summary.EntropyBits,
		summary.HeartPresent,
		summary.ContactConfidence,
		summary.ClassifierConfidence,
	) {
		return "", corebridge.HeartEvidence{}, validationError("evidence contains non-finite values")
	}
	if metrics.TechniqueType > 2 || summary.ClassifierType != metrics.TechniqueType {
		return "", corebridge.HeartEvidence{}, validationError("technique type is invalid")
	}
	if metrics.PeakAccelMPS2 < 0 || metrics.PeakAccelMPS2 > 200 ||
		metrics.ExecTimeSeconds < 0.05 || metrics.ExecTimeSeconds > 3 ||
		!inUnit(metrics.Precision) || !inUnit(metrics.RhythmScore) ||
		metrics.ComboLen > 5 {
		return "", corebridge.HeartEvidence{}, validationError("metrics are outside physical bounds")
	}
	if summary.DurationMS < 5000 || summary.DurationMS > 8000 ||
		summary.MonotonicEndMS <= summary.MonotonicStartMS ||
		summary.MonotonicEndMS-summary.MonotonicStartMS != uint64(summary.DurationMS) {
		return "", corebridge.HeartEvidence{}, validationError("recording duration is invalid")
	}
	if summary.AccelSampleCount < 100 || summary.AccelSampleCount > 2000 ||
		summary.GyroSampleCount < 100 || summary.GyroSampleCount > 2000 ||
		summary.HeartSampleCount < 5 || summary.HeartSampleCount > 1000 {
		return "", corebridge.HeartEvidence{}, validationError("sample counts are implausible")
	}
	if summary.AccelPeakMPS2 < 0 || summary.AccelPeakMPS2 > 200 ||
		summary.AccelRMSMPS2 < 0 || summary.AccelRMSMPS2 > summary.AccelPeakMPS2 ||
		summary.GyroPeakRadiansS < 0 || summary.GyroPeakRadiansS > 40 ||
		summary.GyroRMSRadiansS < 0 || summary.GyroRMSRadiansS > summary.GyroPeakRadiansS ||
		summary.EntropyBits < 0 || summary.EntropyBits > 4 ||
		summary.ZeroCrossings > 5000 {
		return "", corebridge.HeartEvidence{}, validationError("feature summary is outside physical bounds")
	}
	if abs(float64(summary.AccelPeakMPS2-metrics.PeakAccelMPS2)) > 0.5 {
		return "", corebridge.HeartEvidence{}, validationError("peak acceleration fields disagree")
	}
	if !inUnit(heart.Present) || !inUnit(heart.ContactConfidence) ||
		heart.MeanBPM < 35 || heart.MeanBPM > 240 ||
		summary.HeartMeanBPM != heart.MeanBPM ||
		summary.HeartDeltaBPM != heart.DeltaBPM ||
		abs(float64(summary.HeartPresent-heart.Present)) > 0.01 ||
		abs(float64(summary.ContactConfidence-heart.ContactConfidence)) > 0.01 {
		return "", corebridge.HeartEvidence{}, validationError("heart evidence fields disagree")
	}
	if summary.ClassifierID == "" || len(summary.ClassifierID) > 64 ||
		!inUnit(summary.ClassifierConfidence) {
		return "", corebridge.HeartEvidence{}, validationError("classifier summary is invalid")
	}
	recordedAt := time.UnixMilli(request.RecordedAtMS)
	if recordedAt.Before(nonce.IssuedAt.Add(-30*time.Second)) ||
		recordedAt.After(now.Add(30*time.Second)) ||
		recordedAt.After(nonce.ExpiresAt) {
		return "", corebridge.HeartEvidence{}, validationError("recording timestamp is invalid")
	}
	baseline := int32(heart.MeanBPM) - int32(heart.DeltaBPM)
	if baseline < 0 || baseline > 65535 {
		return "", corebridge.HeartEvidence{}, validationError("derived heart baseline is invalid")
	}
	verdict := "VALID"
	if summary.EntropyBits < 2.5 || summary.ClassifierConfidence < 0.65 {
		verdict = "SUSPECT"
	}
	return verdict, corebridge.HeartEvidence{
		BaselineBPM: uint16(baseline),
		MeanBPM:     heart.MeanBPM,
		Present:     heart.Present,
		Confidence:  heart.ContactConfidence,
		DeltaBPM:    heart.DeltaBPM,
	}, nil
}

func toCoreMetrics(metrics Metrics) corebridge.Metrics {
	return corebridge.Metrics{
		PeakAccelMPS2:   metrics.PeakAccelMPS2,
		ExecTimeSeconds: metrics.ExecTimeSeconds,
		Precision:       metrics.Precision,
		ComboLen:        metrics.ComboLen,
		RhythmScore:     metrics.RhythmScore,
		TechniqueType:   metrics.TechniqueType,
	}
}

func replayHash(request SubmitRequest) ([32]byte, error) {
	payload := struct {
		Nonce                 string         `json:"nonce"`
		EvidenceSchemaVersion uint16         `json:"evidenceSchemaVersion"`
		Metrics               Metrics        `json:"metrics"`
		HeartEvidence         HeartEvidence  `json:"heartEvidence"`
		FeatureSummary        FeatureSummary `json:"featureSummary"`
	}{
		Nonce:                 request.Nonce,
		EvidenceSchemaVersion: request.EvidenceSchemaVersion,
		Metrics:               request.Metrics,
		HeartEvidence:         request.HeartEvidence,
		FeatureSummary:        request.FeatureSummary,
	}
	return hashJSON(payload)
}

func hashJSON(value any) ([32]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func validateIdentity(playerID, deviceID string) error {
	if strings.TrimSpace(playerID) == "" || strings.TrimSpace(deviceID) == "" ||
		len(playerID) > maxStringLength || len(deviceID) > maxStringLength {
		return apiError("identity_invalid", "player and device are required", http.StatusBadRequest)
	}
	return nil
}

func validateUUID(value string) error {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return errors.New("invalid UUID")
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded := make([]byte, 16)
	_, err := hex.Decode(decoded, []byte(compact))
	return err
}

func randomToken(reader io.Reader, length int) (string, error) {
	value := make([]byte, length)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
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

func allFinite(values ...float32) bool {
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

func inUnit(value float32) bool {
	return value >= 0 && value <= 1
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func validationError(message string) *Error {
	return apiError("evidence_invalid", message, http.StatusUnprocessableEntity)
}

func nonceInvalidError() *Error {
	return apiError("nonce_invalid", "nonce is unknown or expired", http.StatusConflict)
}

func replayError() *Error {
	return apiError("replay_detected", "nonce or evidence was already used", http.StatusConflict)
}

func idempotencyConflictError() *Error {
	return apiError(
		"idempotency_conflict",
		"Idempotency-Key was already used for another request",
		http.StatusConflict,
	)
}

func coreError(err error) *Error {
	status := http.StatusInternalServerError
	code := "core_error"
	message := "core calculation failed"
	if errors.Is(err, corebridge.ErrUnavailable) {
		status = http.StatusServiceUnavailable
		code = "core_unavailable"
		message = "core calculation is unavailable"
	}
	return &Error{Code: code, Message: message, HTTPStatus: status, Cause: err}
}

func mapCommitError(err error) *Error {
	switch {
	case errors.Is(err, ErrNonceNotFound):
		return nonceInvalidError()
	case errors.Is(err, ErrNonceUsed), errors.Is(err, ErrReplayDetected):
		return replayError()
	case errors.Is(err, ErrIdempotencyConflict):
		return idempotencyConflictError()
	case errors.Is(err, ErrSubmissionRate):
		return apiError(
			"rate_limited",
			"wait at least one minute between Dojo submissions",
			http.StatusTooManyRequests,
		)
	case errors.Is(err, ErrDailyLimit):
		return apiError(
			"daily_limit",
			"daily Dojo submission limit reached",
			http.StatusTooManyRequests,
		)
	default:
		return asAPIError(err)
	}
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for value := range source {
		result[value] = struct{}{}
	}
	return result
}
