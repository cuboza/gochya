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
	if err := engine.VerifyABI(context.Background()); err != nil {
		t.Fatalf("VerifyABI: %v", err)
	}
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
	if difference := stats.BaseDamage - 104; difference < -0.00001 || difference > 0.00001 {
		t.Fatalf("base damage = %f", stats.BaseDamage)
	}
	loot, err := engine.GenerateLootTechnique(context.Background(), 42, 2)
	if err != nil {
		t.Fatalf("GenerateLootTechnique: %v", err)
	}
	repeatedLoot, err := engine.GenerateLootTechnique(context.Background(), 42, 2)
	if err != nil {
		t.Fatalf("repeat GenerateLootTechnique: %v", err)
	}
	if !reflect.DeepEqual(loot, repeatedLoot) ||
		loot.TechniqueType != 5 ||
		loot.Rarity != 0 ||
		loot.Quality != 35 ||
		loot.BaseDamage != 126.5 {
		t.Fatalf("loot = %#v, repeated = %#v", loot, repeatedLoot)
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

func TestNativeEngineBreedingMatchesRustCore(t *testing.T) {
	engine := NativeEngine{}
	input := BreedInput{
		ParentA:          goldenGenome(0, 2, 1),
		ParentB:          goldenGenome(1, 5, 2),
		MutationCatalyst: true,
		HybridCatalyst:   true,
	}
	first, err := engine.Breed(context.Background(), input, 42)
	if err != nil {
		t.Fatalf("Breed: %v", err)
	}
	second, err := engine.Breed(context.Background(), input, 42)
	if err != nil {
		t.Fatalf("repeat Breed: %v", err)
	}
	if !reflect.DeepEqual(first, second) ||
		first.Genome.Generation != 6 ||
		first.IncubationHours < 4 ||
		first.IncubationHours > 24 ||
		first.MutatedGenes&^uint16(0x3fff) != 0 {
		t.Fatalf("breed result = %#v, repeated = %#v", first, second)
	}
	input.ParentA.Visual.PaletteHue = 361
	if _, err := engine.Breed(context.Background(), input, 42); err == nil {
		t.Fatal("Breed accepted invalid parent genome")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Breed(ctx, BreedInput{}, 42); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Breed error = %v", err)
	}
}

func TestNativeEngineStarterGenomeMatchesRustCore(t *testing.T) {
	engine := NativeEngine{}
	first, err := engine.GenerateStarterGenome(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("GenerateStarterGenome: %v", err)
	}
	second, err := engine.GenerateStarterGenome(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("repeat GenerateStarterGenome: %v", err)
	}
	if !reflect.DeepEqual(first, second) ||
		first.Element != 1 ||
		first.Visual.BodyShape != 1 ||
		first.Visual.PaletteHue != 195 ||
		first.Rarity != 0 ||
		first.Ability != 0 ||
		first.Generation != 0 {
		t.Fatalf("starter = %#v, repeated = %#v", first, second)
	}
	if _, err := engine.GenerateStarterGenome(context.Background(), 3, 42); err == nil {
		t.Fatal("GenerateStarterGenome accepted a non-starter element")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.GenerateStarterGenome(ctx, 0, 42); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GenerateStarterGenome error = %v", err)
	}
}

func TestNativeEngineNeedsAndCareMatchRustCore(t *testing.T) {
	engine := NativeEngine{}
	initial := NeedsState{
		Needs: Needs{
			Hunger:  100,
			Energy:  100,
			Hygiene: 100,
			Mood:    100,
		},
	}
	whole, err := engine.AdvanceNeeds(context.Background(), initial, 86_400)
	if err != nil {
		t.Fatalf("AdvanceNeeds: %v", err)
	}
	chunked := initial
	for range 24 {
		chunked, err = engine.AdvanceNeeds(context.Background(), chunked, 3_600)
		if err != nil {
			t.Fatalf("chunked AdvanceNeeds: %v", err)
		}
	}
	if !reflect.DeepEqual(whole, chunked) ||
		whole.Needs != (Needs{Hunger: 76, Energy: 84, Hygiene: 88, Mood: 100}) {
		t.Fatalf("whole = %#v, chunked = %#v", whole, chunked)
	}
	careInput := initial
	careInput.Needs.Hunger = 50
	careInput.Needs.Mood = 90
	fed, err := engine.ApplyCare(context.Background(), careInput, 0, 2)
	if err != nil {
		t.Fatalf("ApplyCare: %v", err)
	}
	if fed.Needs.Hunger != 100 || fed.Needs.Mood != 95 || fed.Sleeping {
		t.Fatalf("fed state = %#v", fed)
	}
	sleeping, err := engine.ApplyCare(context.Background(), careInput, 3, 0)
	if err != nil {
		t.Fatalf("sleep ApplyCare: %v", err)
	}
	if !sleeping.Sleeping {
		t.Fatalf("sleeping state = %#v", sleeping)
	}
	if _, err := engine.ApplyCare(context.Background(), careInput, 0, 4); err == nil {
		t.Fatal("ApplyCare accepted soap as food")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.AdvanceNeeds(ctx, initial, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled AdvanceNeeds error = %v", err)
	}
}

func goldenGenome(element uint8, generation uint32, offset uint8) Genome {
	return Genome{
		Visual: VisualGenes{
			BodyShape:  offset,
			PaletteHue: 30 + uint16(offset),
			PaletteSat: 60 + offset,
			Pattern:    offset,
			Size:       offset,
			EyeStyle:   offset,
			Aura:       offset,
		},
		Stats: StatPotentials{
			Strength:  50 + offset,
			Agility:   60 + offset,
			Endurance: 70 + offset,
			Focus:     80 + offset,
		},
		Element:      element,
		TechAffinity: 1,
		Rarity:       2,
		Ability:      1,
		Generation:   generation,
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
