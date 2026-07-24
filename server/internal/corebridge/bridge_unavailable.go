//go:build !cgo || !gochya_core

package corebridge

import "context"

func (NativeEngine) ValidateHeart(context.Context, HeartEvidence) (HeartVerdict, error) {
	return HeartVerdict{}, ErrUnavailable
}

func (NativeEngine) DeriveTechnique(
	context.Context,
	Metrics,
	HeartEvidence,
	float32,
) (TechniqueStats, error) {
	return TechniqueStats{}, ErrUnavailable
}

func (NativeEngine) SimulateCombat(
	context.Context,
	CombatMatch,
	uint64,
) (CombatResult, error) {
	return CombatResult{}, ErrUnavailable
}

func (NativeEngine) ComputeActivity(
	context.Context,
	ActivitySnapshot,
	ActivityGoals,
	uint32,
) (ActivityResult, error) {
	return ActivityResult{}, ErrUnavailable
}
