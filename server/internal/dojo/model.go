package dojo

import (
	"crypto/ed25519"
	"time"
)

const EvidenceSchemaV1 uint16 = 1

type PreflightRequest struct {
	DeviceID string `json:"deviceId"`
	AppBuild string `json:"appBuild"`
}

type PreflightResponse struct {
	Nonce                 string    `json:"nonce"`
	Challenge             string    `json:"challenge"`
	TraceID               string    `json:"traceId"`
	EvidenceSchemaVersion uint16    `json:"evidenceSchemaVersion"`
	ExpiresAt             time.Time `json:"expiresAt"`
}

type Metrics struct {
	PeakAccelMPS2   float32 `json:"peakAccelMps2"`
	ExecTimeSeconds float32 `json:"execTimeSeconds"`
	Precision       float32 `json:"precision"`
	ComboLen        uint8   `json:"comboLen"`
	RhythmScore     float32 `json:"rhythmScore"`
	TechniqueType   uint8   `json:"techniqueType"`
}

type HeartEvidence struct {
	Present           float32 `json:"present"`
	MeanBPM           uint16  `json:"meanBpm"`
	DeltaBPM          int16   `json:"deltaBpm"`
	ContactConfidence float32 `json:"contactConfidence"`
}

type FeatureSummary struct {
	AccelSampleCount     uint32  `json:"accelSampleCount"`
	GyroSampleCount      uint32  `json:"gyroSampleCount"`
	HeartSampleCount     uint32  `json:"heartSampleCount"`
	DurationMS           uint32  `json:"durationMs"`
	MonotonicStartMS     uint64  `json:"monotonicStartMs"`
	MonotonicEndMS       uint64  `json:"monotonicEndMs"`
	AccelPeakMPS2        float32 `json:"accelPeakMps2"`
	AccelRMSMPS2         float32 `json:"accelRmsMps2"`
	GyroPeakRadiansS     float32 `json:"gyroPeakRadiansS"`
	GyroRMSRadiansS      float32 `json:"gyroRmsRadiansS"`
	EntropyBits          float32 `json:"entropyBits"`
	ZeroCrossings        uint32  `json:"zeroCrossings"`
	HeartMeanBPM         uint16  `json:"heartMeanBpm"`
	HeartDeltaBPM        int16   `json:"heartDeltaBpm"`
	HeartPresent         float32 `json:"heartPresent"`
	ContactConfidence    float32 `json:"contactConfidence"`
	ClassifierID         string  `json:"classifierId"`
	ClassifierType       uint8   `json:"classifierType"`
	ClassifierConfidence float32 `json:"classifierConfidence"`
}

type AttestationEvidence struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

type SubmitRequest struct {
	DeviceID              string              `json:"deviceId"`
	Nonce                 string              `json:"nonce"`
	EvidenceSchemaVersion uint16              `json:"evidenceSchemaVersion"`
	RecordedAtMS          int64               `json:"recordedAtMs"`
	Metrics               Metrics             `json:"metrics"`
	HeartEvidence         HeartEvidence       `json:"heartEvidence"`
	FeatureSummary        FeatureSummary      `json:"featureSummary"`
	ClassifierVersion     string              `json:"classifierVersion"`
	AppBuild              string              `json:"appBuild"`
	Attestation           AttestationEvidence `json:"attestation"`
	PayloadSignature      string              `json:"payloadSignature"`
}

type TechniqueCard struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"ownerId"`
	Type        uint8     `json:"type"`
	Element     uint8     `json:"element"`
	Rarity      uint8     `json:"rarity"`
	BaseDamage  float32   `json:"baseDamage"`
	Speed       float32   `json:"speed"`
	StaminaCost uint16    `json:"staminaCost"`
	CritChance  float32   `json:"critChance"`
	Effect      uint8     `json:"effect"`
	Quality     uint8     `json:"quality"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SubmitResponse struct {
	Card            TechniqueCard `json:"card"`
	EvidenceVerdict string        `json:"evidenceVerdict"`
	TraceID         string        `json:"traceId"`
}

type Device struct {
	ID        string
	PlayerID  string
	PublicKey ed25519.PublicKey
	Enabled   bool
}

type NonceRecord struct {
	Value                 string
	Challenge             string
	TraceID               string
	PlayerID              string
	DeviceID              string
	AppBuild              string
	EvidenceSchemaVersion uint16
	IssuedAt              time.Time
	ExpiresAt             time.Time
	UsedAt                *time.Time
}
