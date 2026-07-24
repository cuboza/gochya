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

const MaxCombatRounds = 20

type CombatStats struct {
	Strength  uint32
	Agility   uint32
	Endurance uint32
	Focus     uint32
}

type CombatGear struct {
	StrengthBonus  int16
	AgilityBonus   int16
	EnduranceBonus int16
	FocusBonus     int16
}

type CombatCard struct {
	BaseDamage    float32
	Speed         float32
	CritChance    float32
	EffectValue   float32
	StaminaCost   uint16
	TechniqueType uint8
	EffectKind    uint8
}

type CombatLoadout struct {
	Stats        CombatStats
	Gear         CombatGear
	Element      uint8
	TechAffinity uint8
	PetMood      uint8
	SignatureIdx uint8
	Cards        [5]CombatCard
}

type CombatMatch struct {
	LoadoutA CombatLoadout
	LoadoutB CombatLoadout
	Mode     uint8
}

type CombatRound struct {
	CardAIdx    uint8
	CardBIdx    uint8
	DamageAToB  uint16
	DamageBToA  uint16
	EffectKind  uint8
	EffectValue float32
}

type CombatResult struct {
	Winner   uint8
	Rounds   []CombatRound
	FinalHPA uint16
	FinalHPB uint16
	Seed     uint64
}

type Engine interface {
	ValidateHeart(context.Context, HeartEvidence) (HeartVerdict, error)
	DeriveTechnique(context.Context, Metrics, HeartEvidence, float32) (TechniqueStats, error)
}

type CombatEngine interface {
	SimulateCombat(context.Context, CombatMatch, uint64) (CombatResult, error)
}

type NativeEngine struct{}
