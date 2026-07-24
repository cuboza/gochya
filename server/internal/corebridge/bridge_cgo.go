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
	"math"
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

func (NativeEngine) SimulateCombat(
	ctx context.Context,
	match CombatMatch,
	seed uint64,
) (CombatResult, error) {
	select {
	case <-ctx.Done():
		return CombatResult{}, ctx.Err()
	default:
	}
	input := C.GochyaCombatMatchV1{}
	input.struct_size = C.uint32_t(unsafe.Sizeof(input))
	input.schema_version = schemaVersion
	input.mode = C.uint8_t(match.Mode)
	input.loadout_a = combatLoadoutInput(match.LoadoutA)
	input.loadout_b = combatLoadoutInput(match.LoadoutB)

	var output C.GochyaCombatResultV1
	status := C.gochya_simulate_combat_v1(
		&input,
		C.uint64_t(seed),
		&output,
	)
	if status != C.GochyaStatus_Ok {
		return CombatResult{}, fmt.Errorf(
			"simulate combat: core status %d",
			int32(status),
		)
	}
	if output.struct_size != C.uint32_t(unsafe.Sizeof(output)) ||
		output.schema_version != schemaVersion ||
		output.winner > 2 ||
		output.round_count > MaxCombatRounds ||
		uint64(output.seed) != seed {
		return CombatResult{}, fmt.Errorf("simulate combat: invalid core output envelope")
	}
	result := CombatResult{
		Winner:   uint8(output.winner),
		Rounds:   make([]CombatRound, int(output.round_count)),
		FinalHPA: uint16(output.final_hp_a),
		FinalHPB: uint16(output.final_hp_b),
		Seed:     uint64(output.seed),
	}
	for index := range result.Rounds {
		round := output.rounds[index]
		effectValue := float32(round.effect_value)
		if round.card_a_idx > 4 ||
			round.card_b_idx > 4 ||
			round.effect_kind > 5 ||
			math.IsNaN(float64(effectValue)) ||
			math.IsInf(float64(effectValue), 0) {
			return CombatResult{}, fmt.Errorf(
				"simulate combat: invalid core round %d",
				index,
			)
		}
		result.Rounds[index] = CombatRound{
			CardAIdx:    uint8(round.card_a_idx),
			CardBIdx:    uint8(round.card_b_idx),
			DamageAToB:  uint16(round.damage_a_to_b),
			DamageBToA:  uint16(round.damage_b_to_a),
			EffectKind:  uint8(round.effect_kind),
			EffectValue: effectValue,
		}
	}
	return result, nil
}

func combatLoadoutInput(loadout CombatLoadout) C.GochyaCombatLoadoutV1 {
	input := C.GochyaCombatLoadoutV1{}
	input.stat_str = C.uint32_t(loadout.Stats.Strength)
	input.stat_agi = C.uint32_t(loadout.Stats.Agility)
	input.stat_end = C.uint32_t(loadout.Stats.Endurance)
	input.stat_foc = C.uint32_t(loadout.Stats.Focus)
	input.gear_str_bonus = C.int16_t(loadout.Gear.StrengthBonus)
	input.gear_agi_bonus = C.int16_t(loadout.Gear.AgilityBonus)
	input.gear_end_bonus = C.int16_t(loadout.Gear.EnduranceBonus)
	input.gear_foc_bonus = C.int16_t(loadout.Gear.FocusBonus)
	input.element = C.uint8_t(loadout.Element)
	input.tech_affinity = C.uint8_t(loadout.TechAffinity)
	input.pet_mood = C.uint8_t(loadout.PetMood)
	input.signature_idx = C.uint8_t(loadout.SignatureIdx)
	for index, card := range loadout.Cards {
		input.cards[index].base_damage = C.float(card.BaseDamage)
		input.cards[index].speed = C.float(card.Speed)
		input.cards[index].crit_chance = C.float(card.CritChance)
		input.cards[index].effect_value = C.float(card.EffectValue)
		input.cards[index].stamina_cost = C.uint16_t(card.StaminaCost)
		input.cards[index].technique_type = C.uint8_t(card.TechniqueType)
		input.cards[index].effect_kind = C.uint8_t(card.EffectKind)
	}
	return input
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
