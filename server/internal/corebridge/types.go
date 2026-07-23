package corebridge

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("gochya core is unavailable")

type Metrics struct {
	PeakAccelMPS2   float32
	ExecTimeSeconds float32
	Precision       float32
	ComboLen        uint8
	RhythmScore     float32
	TechniqueType   uint8
}

type HeartEvidence struct {
	BaselineBPM uint16
	MeanBPM     uint16
	Present     float32
	Confidence  float32
	DeltaBPM    int16
}

type HeartVerdict struct {
	Passed     bool
	Reason     uint8
	HeartScore float32
}

type TechniqueStats struct {
	TechniqueType uint8
	Rarity        uint8
	BaseDamage    float32
	Speed         float32
	StaminaCost   uint16
	CritChance    float32
	Quality       uint8
}

type Engine interface {
	ValidateHeart(context.Context, HeartEvidence) (HeartVerdict, error)
	DeriveTechnique(context.Context, Metrics, HeartEvidence, float32) (TechniqueStats, error)
}

type NativeEngine struct{}
