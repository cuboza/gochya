#include "gochya_core.h"

#include <assert.h>
#include <math.h>
#include <stdint.h>
#include <string.h>

int main(void) {
  assert(gochya_abi_version() == UINT32_C(0x00010000));
  assert(sizeof(GochyaPunchMetricsV1) == 40);
  assert(sizeof(GochyaHeartEvidenceV1) == 36);
  assert(sizeof(GochyaHeartVerdictV1) == 28);

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

  return 0;
}
