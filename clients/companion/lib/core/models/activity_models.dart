import 'profile_models.dart';
import 'technique_models.dart';

/// `MAX_VITALITY_PER_DAY` from `core/src/synergy.rs`.
const maxVitalityPerDay = 150;

/// `ActivityRewardVitality` from `server/internal/activity/model.go`.
const activityRewardVitality = 100;

class ActivityGoals {
  const ActivityGoals({
    required this.steps,
    required this.sleepHours,
    required this.activeCalories,
  });

  factory ActivityGoals.fromJson(JsonMap json) {
    return ActivityGoals(
      steps: rangedInt(json, 'steps', min: 0),
      sleepHours: requiredDouble(json, 'sleepHours', min: 0, max: 24),
      activeCalories: rangedInt(json, 'activeCalories', min: 0, max: 65535),
    );
  }

  final int steps;
  final double sleepHours;
  final int activeCalories;
}

class ActivityStatGains {
  const ActivityStatGains({
    required this.strength,
    required this.agility,
    required this.endurance,
    required this.focus,
  });

  factory ActivityStatGains.fromJson(JsonMap json) {
    return ActivityStatGains(
      strength: rangedInt(json, 'str', min: -32768, max: 32767),
      agility: rangedInt(json, 'agi', min: -32768, max: 32767),
      endurance: rangedInt(json, 'end', min: -32768, max: 32767),
      focus: rangedInt(json, 'foc', min: -32768, max: 32767),
    );
  }

  final int strength;
  final int agility;
  final int endurance;
  final int focus;

  int get total => strength + agility + endurance + focus;
}

/// The subset of the stored snapshot the phone renders. Raw sensor series never
/// reach the server, so a day only carries derived aggregates.
class ActivitySnapshotSummary {
  const ActivitySnapshotSummary({
    required this.steps,
    required this.sleepMinutes,
    required this.activeCalories,
    required this.workouts,
  });

  factory ActivitySnapshotSummary.fromJson(JsonMap json) {
    return ActivitySnapshotSummary(
      steps: rangedInt(json, 'steps', min: 0),
      sleepMinutes: rangedInt(json, 'sleepMinutes', min: 0, max: 65535),
      activeCalories: rangedInt(json, 'activeCalories', min: 0, max: 65535),
      workouts: requiredList(json, 'workouts').length,
    );
  }

  final int steps;
  final int sleepMinutes;
  final int activeCalories;
  final int workouts;

  double get sleepHours => sleepMinutes / 60;
}

class DailyActivity {
  const DailyActivity({
    required this.date,
    required this.snapshot,
    required this.vitality,
    required this.vitalityAwarded,
    required this.statGains,
    required this.goals,
    required this.sourceMetadata,
    required this.updatedAt,
  });

  factory DailyActivity.fromJson(JsonMap json) {
    final date = requiredString(json, 'date');
    if (!_isCalendarDate(date)) {
      throw FormatException('activity date $date is not a calendar day');
    }
    final vitality = rangedInt(
      json,
      'vitality',
      min: 0,
      max: maxVitalityPerDay,
    );
    final awarded = rangedInt(
      json,
      'vitalityAwarded',
      min: 0,
      max: maxVitalityPerDay,
    );
    if (awarded > vitality) {
      throw const FormatException('awarded vitality exceeds the day total');
    }
    return DailyActivity(
      date: date,
      snapshot: ActivitySnapshotSummary.fromJson(requiredMap(json, 'snapshot')),
      vitality: vitality,
      vitalityAwarded: awarded,
      statGains: ActivityStatGains.fromJson(requiredMap(json, 'statGains')),
      goals: ActivityGoals.fromJson(requiredMap(json, 'goals')),
      sourceMetadata: requiredString(json, 'sourceMetadata'),
      updatedAt: requiredDateTime(json, 'updatedAt'),
    );
  }

  final String date;
  final ActivitySnapshotSummary snapshot;
  final int vitality;
  final int vitalityAwarded;
  final ActivityStatGains statGains;
  final ActivityGoals goals;
  final String sourceMetadata;
  final DateTime updatedAt;

  bool get unlocksReward => vitality >= activityRewardVitality;

  /// Progress toward the daily Technique Card reward, not toward the cap.
  double get rewardProgress =>
      (vitality / activityRewardVitality).clamp(0, 1).toDouble();
}

/// Deterministic daily Technique Card granted by the server, not the client.
class ActivityRewardResult {
  const ActivityRewardResult({
    required this.date,
    required this.card,
    required this.awarded,
  });

  factory ActivityRewardResult.fromJson(JsonMap json) {
    final date = requiredString(json, 'date');
    if (!_isCalendarDate(date)) {
      throw FormatException('reward date $date is not a calendar day');
    }
    return ActivityRewardResult(
      date: date,
      card: TechniqueCardSummary.fromJson(requiredMap(json, 'card')),
      awarded: requiredBool(json, 'awarded'),
    );
  }

  final String date;
  final TechniqueCardSummary card;

  /// `false` when an earlier claim already granted the same card.
  final bool awarded;
}

final _calendarDate = RegExp(r'^\d{4}-\d{2}-\d{2}$');

bool _isCalendarDate(String value) {
  return _calendarDate.hasMatch(value) && DateTime.tryParse(value) != null;
}
