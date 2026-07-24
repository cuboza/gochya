//go:build cgo && gochya_core

package corebridge

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestNativeEngineCallsRustCore(t *testing.T) {
	engine := NativeEngine{}
	heart := HeartEvidence{
		BaselineBPM: 70,
		MeanBPM:     90,
		Present:     0.9,
		Confidence:  0.9,
		DeltaBPM:    20,
	}
	verdict, err := engine.ValidateHeart(context.Background(), heart)
	if err != nil {
		t.Fatalf("ValidateHeart: %v", err)
	}
	if !verdict.Passed {
		t.Fatalf("heart verdict = %#v", verdict)
	}
	stats, err := engine.DeriveTechnique(context.Background(), Metrics{
		PeakAccelMPS2:   65,
		ExecTimeSeconds: 0.5,
		Precision:       0.8,
		ComboLen:        3,
		RhythmScore:     0.75,
		TechniqueType:   1,
	}, heart, 1)
	if err != nil {
		t.Fatalf("DeriveTechnique: %v", err)
	}
	if stats.TechniqueType != 1 || stats.Rarity != 2 || stats.Quality != 64 ||
		stats.StaminaCost != 3 {
		t.Fatalf("stats = %#v", stats)
	}
	if difference := stats.BaseDamage - 1.04; difference < -0.00001 || difference > 0.00001 {
		t.Fatalf("base damage = %f", stats.BaseDamage)
	}
}

func TestNativeEngineActivityMatchesRustGoldenFixture(t *testing.T) {
	engine := NativeEngine{}
	goals, err := engine.ComputeGoals(context.Background(), ActivityBaseline{
		StepsAverage:          8_000,
		SleepHoursAverage:     7,
		ActiveCaloriesAverage: 400,
	})
	if err != nil {
		t.Fatalf("ComputeGoals: %v", err)
	}
	if goals.Steps != 9_200 ||
		goals.ActiveCalories != 460 ||
		goals.SleepHours < 7.69999 ||
		goals.SleepHours > 7.70001 {
		t.Fatalf("activity goals = %#v", goals)
	}
	result, err := engine.ComputeActivity(
		context.Background(),
		goldenActivitySnapshot(),
		ActivityGoals{
			Steps:          10_000,
			SleepHours:     8,
			ActiveCalories: 500,
		},
		10,
	)
	if err != nil {
		t.Fatalf("ComputeActivity: %v", err)
	}
	expected := ActivityResult{
		Vitality: 104,
		StatGains: ActivityStatGains{
			Strength:  7,
			Agility:   7,
			Endurance: 12,
			Focus:     7,
		},
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("activity result = %#v, want %#v", result, expected)
	}
}

func TestNativeEngineActivityRejectsInvalidInputAndCancellation(t *testing.T) {
	engine := NativeEngine{}
	snapshot := goldenActivitySnapshot()
	snapshot.Source = 2
	if _, err := engine.ComputeActivity(
		context.Background(),
		snapshot,
		ActivityGoals{SleepHours: 8},
		10,
	); err == nil {
		t.Fatal("ComputeActivity accepted invalid source")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot.Source = 0
	if _, err := engine.ComputeActivity(
		ctx,
		snapshot,
		ActivityGoals{SleepHours: 8},
		10,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ComputeActivity error = %v", err)
	}
}

func TestNativeEngineCombatMatchesRustGoldenFixture(t *testing.T) {
	engine := NativeEngine{}
	result, err := engine.SimulateCombat(
		context.Background(),
		CombatMatch{
			LoadoutA: goldenCombatLoadout(0, 260, 70),
			LoadoutB: goldenCombatLoadout(2, 240, 60),
			Mode:     0,
		},
		42,
	)
	if err != nil {
		t.Fatalf("SimulateCombat: %v", err)
	}
	expectedRounds := []CombatRound{
		{CardAIdx: 0, CardBIdx: 0, DamageAToB: 437, DamageBToA: 169},
		{CardAIdx: 0, CardBIdx: 0, DamageAToB: 453, DamageBToA: 181},
		{CardAIdx: 0, CardBIdx: 0, DamageAToB: 413, DamageBToA: 0},
	}
	if result.Winner != 0 ||
		result.FinalHPA != 950 ||
		result.FinalHPB != 0 ||
		result.Seed != 42 ||
		!reflect.DeepEqual(result.Rounds, expectedRounds) {
		t.Fatalf("combat result = %#v", result)
	}
	repeated, err := engine.SimulateCombat(
		context.Background(),
		CombatMatch{
			LoadoutA: goldenCombatLoadout(0, 260, 70),
			LoadoutB: goldenCombatLoadout(2, 240, 60),
			Mode:     0,
		},
		42,
	)
	if err != nil {
		t.Fatalf("repeat SimulateCombat: %v", err)
	}
	if !reflect.DeepEqual(repeated, result) {
		t.Fatalf("repeated result = %#v, want %#v", repeated, result)
	}
}

func TestNativeEngineCombatRejectsInvalidInputAndCancellation(t *testing.T) {
	engine := NativeEngine{}
	match := CombatMatch{
		LoadoutA: goldenCombatLoadout(0, 260, 70),
		LoadoutB: goldenCombatLoadout(2, 240, 60),
	}
	match.LoadoutA.SignatureIdx = 5
	if _, err := engine.SimulateCombat(context.Background(), match, 42); err == nil {
		t.Fatal("SimulateCombat accepted invalid signature index")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	match.LoadoutA.SignatureIdx = 4
	if _, err := engine.SimulateCombat(ctx, match, 42); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SimulateCombat error = %v", err)
	}
}

func goldenCombatLoadout(
	element uint8,
	baseDamage float32,
	speed float32,
) CombatLoadout {
	card := CombatCard{
		BaseDamage:    baseDamage,
		Speed:         speed,
		StaminaCost:   10,
		TechniqueType: 0,
	}
	return CombatLoadout{
		Stats: CombatStats{
			Strength:  30,
			Agility:   30,
			Endurance: 30,
			Focus:     30,
		},
		Element:      element,
		TechAffinity: 0,
		PetMood:      100,
		SignatureIdx: 4,
		Cards:        [5]CombatCard{card, card, card, card, card},
	}
}

func goldenActivitySnapshot() ActivitySnapshot {
	return ActivitySnapshot{
		Steps:          10_000,
		SleepMinutes:   480,
		SleepQuality:   100,
		ActiveCalories: 500,
		Workouts: [MaxActivityWorkouts]ActivityWorkout{
			{Kind: 2, DurationMinutes: 30, Calories: 150},
			{Kind: 0, DurationMinutes: 30, Calories: 200},
			{Kind: 4, DurationMinutes: 60, Calories: 150},
		},
		WorkoutCount:         3,
		HighHeartZoneMinutes: 10,
		MeditationMinutes:    15,
		StressLevel:          20,
		Floors:               10,
		Source:               0,
		PetElement:           2,
	}
}
