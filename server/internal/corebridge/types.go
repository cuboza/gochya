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

const MaxActivityWorkouts = 8

type ActivityWorkout struct {
	Kind            uint8
	DurationMinutes uint16
	Calories        uint16
}

type ActivitySnapshot struct {
	Steps                uint32
	SleepMinutes         uint16
	SleepQuality         uint8
	ActiveCalories       uint16
	Workouts             [MaxActivityWorkouts]ActivityWorkout
	WorkoutCount         uint8
	AverageHeartRate     uint16
	HighHeartZoneMinutes uint16
	MeditationMinutes    uint16
	StressLevel          uint8
	Floors               uint16
	StandHours           uint8
	Source               uint8
	Timestamp            uint64
	PetElement           uint8
}

type ActivityGoals struct {
	Steps          uint32
	SleepHours     float32
	ActiveCalories uint16
}

type ActivityBaseline struct {
	StepsAverage          uint32
	SleepHoursAverage     float32
	ActiveCaloriesAverage uint16
}

type ActivityStatGains struct {
	Strength  int16
	Agility   int16
	Endurance int16
	Focus     int16
}

type ActivityResult struct {
	Vitality  uint16
	StatGains ActivityStatGains
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

type ActivityEngine interface {
	ComputeGoals(context.Context, ActivityBaseline) (ActivityGoals, error)
	ComputeActivity(
		context.Context,
		ActivitySnapshot,
		ActivityGoals,
		uint32,
	) (ActivityResult, error)
}

type NativeEngine struct{}
