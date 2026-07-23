//go:build cgo && gochya_core

package corebridge

import (
	"context"
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
