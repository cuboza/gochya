//go:build cgo && gochya_core

package corebridge

/*
#cgo CFLAGS: -I${SRCDIR}/../../../core/ffi
#cgo LDFLAGS: ${SRCDIR}/../../../target/release/libgochya_core.a -lpthread -ldl -lm
#include "gochya_core.h"
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

const schemaVersion = 1

func (NativeEngine) ValidateHeart(_ context.Context, heart HeartEvidence) (HeartVerdict, error) {
	input := heartInput(heart)
	var output C.GochyaHeartVerdictV1
	status := C.gochya_validate_heart_v1(&input, &output)
	if status != C.GochyaStatus_Ok {
		return HeartVerdict{}, fmt.Errorf("validate heart: core status %d", int32(status))
	}
	return HeartVerdict{
		Passed:     output.passed != 0,
		Reason:     uint8(output.reason),
		HeartScore: float32(output.heart_score),
	}, nil
}

func (NativeEngine) DeriveTechnique(
	_ context.Context,
	metrics Metrics,
	heart HeartEvidence,
	techLevel float32,
) (TechniqueStats, error) {
	metricsInput := C.GochyaPunchMetricsV1{}
	metricsInput.struct_size = C.uint32_t(unsafe.Sizeof(metricsInput))
	metricsInput.schema_version = schemaVersion
	metricsInput.technique_type = C.uint8_t(metrics.TechniqueType)
	metricsInput.combo_len = C.uint8_t(metrics.ComboLen)
	metricsInput.peak_accel_mps2 = C.float(metrics.PeakAccelMPS2)
	metricsInput.exec_time_seconds = C.float(metrics.ExecTimeSeconds)
	metricsInput.precision = C.float(metrics.Precision)
	metricsInput.rhythm_score = C.float(metrics.RhythmScore)

	heartValue := heartInput(heart)
	var output C.GochyaTechniqueStatsV1
	status := C.gochya_derive_technique_v1(
		&metricsInput,
		&heartValue,
		C.float(techLevel),
		&output,
	)
	if status != C.GochyaStatus_Ok {
		return TechniqueStats{}, fmt.Errorf("derive technique: core status %d", int32(status))
	}
	return TechniqueStats{
		TechniqueType: uint8(output.technique_type),
		Rarity:        uint8(output.rarity),
		BaseDamage:    float32(output.base_damage),
		Speed:         float32(output.speed),
		StaminaCost:   uint16(output.stamina_cost),
		CritChance:    float32(output.crit_chance),
		Quality:       uint8(output.quality),
	}, nil
}

func heartInput(heart HeartEvidence) C.GochyaHeartEvidenceV1 {
	input := C.GochyaHeartEvidenceV1{}
	input.struct_size = C.uint32_t(unsafe.Sizeof(input))
	input.schema_version = schemaVersion
	input.baseline_bpm = C.uint16_t(heart.BaselineBPM)
	input.mean_bpm = C.uint16_t(heart.MeanBPM)
	input.present = C.float(heart.Present)
	input.confidence = C.float(heart.Confidence)
	input.delta_bpm = C.int16_t(heart.DeltaBPM)
	return input
}
