#include "gochya_core.h"

#include <assert.h>
#include <math.h>
#include <stdint.h>
#include <string.h>

int main(void) {
  assert(gochya_abi_version() == UINT32_C(0x00020000));
  assert(sizeof(GochyaPunchMetricsV1) == 40);
  assert(sizeof(GochyaHeartEvidenceV1) == 36);
  assert(sizeof(GochyaHeartVerdictV1) == 28);
  assert(sizeof(GochyaPersonalBaselineV1) == 32);
  assert(sizeof(GochyaWorkoutV1) == 8);
  assert(sizeof(GochyaActivityInputV1) == 120);
  assert(sizeof(GochyaActivityResultV1) == 32);
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
  assert(fabsf(stats.base_damage - 104.0f) < 0.00001f);
  assert(fabsf(stats.speed - 66.666664f) < 0.00001f);
  assert(stats.stamina_cost == 3);
  assert(fabsf(stats.crit_chance - 0.0625f) < 0.00001f);
  assert(stats.quality == 64);

  GochyaTechniqueStatsV1 loot_stats;
  memset(&loot_stats, 0, sizeof(loot_stats));
  assert(gochya_generate_loot_technique_v1(42, 2, &loot_stats) == GochyaStatus_Ok);
  assert(loot_stats.struct_size == sizeof(loot_stats));
  assert(loot_stats.schema_version == 1);
  assert(loot_stats.technique_type == 5);
  assert(loot_stats.rarity == 0);
  assert(fabsf(loot_stats.base_damage - 126.5f) < 0.00001f);
  assert(loot_stats.quality == 35);
  assert(gochya_generate_loot_technique_v1(42, 4, &loot_stats) ==
         GochyaStatus_InvalidArgument);

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

  GochyaPersonalBaselineV1 baseline;
  memset(&baseline, 0, sizeof(baseline));
  baseline.struct_size = sizeof(baseline);
  baseline.schema_version = 1;
  baseline.steps_14d_average = 8000;
  baseline.sleep_hours_14d_average = 7.0f;
  baseline.active_calories_14d_average = 400;
  GochyaDailyGoalsV1 adaptive_goals;
  memset(&adaptive_goals, 0, sizeof(adaptive_goals));
  assert(gochya_compute_goals_v1(&baseline, &adaptive_goals) == GochyaStatus_Ok);
  assert(adaptive_goals.struct_size == sizeof(adaptive_goals));
  assert(adaptive_goals.steps == 9200);
  assert(fabsf(adaptive_goals.sleep_hours - 7.7f) < 0.00001f);
  assert(adaptive_goals.active_calories == 460);

  GochyaActivityInputV1 activity_input;
  memset(&activity_input, 0, sizeof(activity_input));
  activity_input.struct_size = sizeof(activity_input);
  activity_input.schema_version = 1;
  activity_input.steps = 10000;
  activity_input.sleep_minutes = 480;
  activity_input.active_calories = 500;
  activity_input.sleep_quality = 100;
  activity_input.workout_count = 3;
  activity_input.stress_level = 20;
  activity_input.source = 0;
  activity_input.pet_element = 2;
  activity_input.hr_zone_high_minutes = 10;
  activity_input.meditation_minutes = 15;
  activity_input.floors = 10;
  activity_input.workouts[0].kind = 2;
  activity_input.workouts[0].duration_minutes = 30;
  activity_input.workouts[0].calories = 150;
  activity_input.workouts[1].kind = 0;
  activity_input.workouts[1].duration_minutes = 30;
  activity_input.workouts[1].calories = 200;
  activity_input.workouts[2].kind = 4;
  activity_input.workouts[2].duration_minutes = 60;
  activity_input.workouts[2].calories = 150;
  goals.steps = 10000;
  goals.sleep_hours = 8.0f;
  goals.active_calories = 500;
  GochyaActivityResultV1 activity_result;
  memset(&activity_result, 0, sizeof(activity_result));
  assert(gochya_compute_activity_v1(&activity_input, &goals, 10, &activity_result) ==
         GochyaStatus_Ok);
  assert(activity_result.struct_size == sizeof(activity_result));
  assert(activity_result.schema_version == 1);
  assert(activity_result.vitality == 104);
  assert(activity_result.stat_str == 7);
  assert(activity_result.stat_agi == 7);
  assert(activity_result.stat_end == 12);
  assert(activity_result.stat_foc == 7);

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
