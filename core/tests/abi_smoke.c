#include "gochya_core.h"

#include <assert.h>
#include <math.h>
#include <stdint.h>
#include <string.h>

int main(void) {
  assert(gochya_abi_version() == UINT32_C(0x00010100));
  assert(sizeof(GochyaPunchMetricsV1) == 40);
  assert(sizeof(GochyaHeartEvidenceV1) == 36);
  assert(sizeof(GochyaHeartVerdictV1) == 28);
  assert(sizeof(GochyaCombatCardV1) == 20);
  assert(sizeof(GochyaCombatLoadoutV1) == 144);
  assert(sizeof(GochyaCombatMatchV1) == 312);
  assert(sizeof(GochyaCombatRoundV1) == 12);
  assert(sizeof(GochyaCombatResultV1) == 280);

  GochyaHeartEvidenceV1 heart;
  memset(&heart, 0, sizeof(heart));
  heart.struct_size = sizeof(heart);
  heart.schema_version = 1;
  heart.baseline_bpm = 70;
  heart.mean_bpm = 90;
  heart.present = 0.9f;
  heart.confidence = 0.9f;
  heart.delta_bpm = 20;

  GochyaHeartVerdictV1 verdict;
  memset(&verdict, 0, sizeof(verdict));
  assert(gochya_validate_heart_v1(&heart, &verdict) == GochyaStatus_Ok);
  assert(verdict.passed == 1);
  assert(verdict.reason == 0);
  assert(fabsf(verdict.heart_score - 0.70f) < 0.00001f);

  GochyaPunchMetricsV1 metrics;
  memset(&metrics, 0, sizeof(metrics));
  metrics.struct_size = sizeof(metrics);
  metrics.schema_version = 1;
  metrics.technique_type = 1;
  metrics.combo_len = 3;
  metrics.peak_accel_mps2 = 65.0f;
  metrics.exec_time_seconds = 0.5f;
  metrics.precision = 0.8f;
  metrics.rhythm_score = 0.75f;
  uint8_t score = 0;
  assert(gochya_quality_score_v1(&metrics, &heart, &score) == GochyaStatus_Ok);
  assert(score == 64);

  GochyaTechniqueStatsV1 stats;
  memset(&stats, 0, sizeof(stats));
  assert(gochya_derive_technique_v1(&metrics, &heart, 1.0f, &stats) == GochyaStatus_Ok);
  assert(stats.technique_type == 1);
  assert(stats.rarity == 2);
  assert(fabsf(stats.base_damage - 1.04f) < 0.00001f);
  assert(fabsf(stats.speed - 66.666664f) < 0.00001f);
  assert(stats.stamina_cost == 3);
  assert(fabsf(stats.crit_chance - 0.0625f) < 0.00001f);
  assert(stats.quality == 64);

  GochyaDailyActivityV1 activity;
  memset(&activity, 0, sizeof(activity));
  activity.struct_size = sizeof(activity);
  activity.schema_version = 1;
  activity.steps = 15000;
  activity.sleep_minutes = 624;
  activity.active_calories = 750;
  activity.workout_count = 3;

  GochyaDailyGoalsV1 goals;
  memset(&goals, 0, sizeof(goals));
  goals.struct_size = sizeof(goals);
  goals.schema_version = 1;
  goals.steps = 10000;
  goals.sleep_hours = 8.0f;
  goals.active_calories = 500;
  uint16_t vitality = 0;
  assert(gochya_compute_vitality_v1(&activity, &goals, 32, &vitality) == GochyaStatus_Ok);
  assert(vitality == 150);

  GochyaCombatMatchV1 match;
  memset(&match, 0, sizeof(match));
  match.struct_size = sizeof(match);
  match.schema_version = 1;
  match.mode = 0;
  match.loadout_a.stat_str = 30;
  match.loadout_a.stat_agi = 30;
  match.loadout_a.stat_end = 30;
  match.loadout_a.stat_foc = 30;
  match.loadout_a.element = 0;
  match.loadout_a.tech_affinity = 0;
  match.loadout_a.pet_mood = 100;
  match.loadout_a.signature_idx = 4;
  match.loadout_b.stat_str = 30;
  match.loadout_b.stat_agi = 30;
  match.loadout_b.stat_end = 30;
  match.loadout_b.stat_foc = 30;
  match.loadout_b.element = 2;
  match.loadout_b.tech_affinity = 0;
  match.loadout_b.pet_mood = 100;
  match.loadout_b.signature_idx = 4;
  for (size_t index = 0; index < 5; index++) {
    match.loadout_a.cards[index].base_damage = 260.0f;
    match.loadout_a.cards[index].speed = 70.0f;
    match.loadout_a.cards[index].stamina_cost = 10;
    match.loadout_b.cards[index].base_damage = 240.0f;
    match.loadout_b.cards[index].speed = 60.0f;
    match.loadout_b.cards[index].stamina_cost = 10;
  }
  GochyaCombatResultV1 combat;
  memset(&combat, 0, sizeof(combat));
  assert(gochya_simulate_combat_v1(&match, 42, &combat) == GochyaStatus_Ok);
  assert(combat.struct_size == sizeof(combat));
  assert(combat.schema_version == 1);
  assert(combat.winner == 0);
  assert(combat.round_count == 3);
  assert(combat.final_hp_a == 950);
  assert(combat.final_hp_b == 0);
  assert(combat.seed == 42);
  assert(combat.rounds[0].damage_a_to_b == 437);
  assert(combat.rounds[0].damage_b_to_a == 169);
  assert(combat.rounds[1].damage_a_to_b == 453);
  assert(combat.rounds[1].damage_b_to_a == 181);
  assert(combat.rounds[2].damage_a_to_b == 413);
  assert(combat.rounds[2].damage_b_to_a == 0);

  return 0;
}
