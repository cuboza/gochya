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
