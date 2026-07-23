use serde::{Deserialize, Serialize};

pub const MAX_WORKOUTS: usize = 8;
pub const MAX_WORKOUTS_FOR_GAIN: u8 = 3;
pub const MAX_VITALITY_PER_DAY: u16 = 150;

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum DataSource {
    #[default]
    Watch = 0,
    Phone = 1,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct WorkoutSummary {
    pub kind: u8,
    pub duration_min: u16,
    pub calories: u16,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct DailyActivitySnapshot {
    pub steps: u32,
    pub sleep_minutes: u16,
    pub sleep_quality: u8,
    pub active_calories: u16,
    pub workouts: [WorkoutSummary; MAX_WORKOUTS],
    pub workout_count: u8,
    pub avg_hr: u16,
    pub hr_zone_high_min: u16,
    pub meditation_min: u16,
    pub stress_level: u8,
    pub floors: u16,
    pub stand_hours: u8,
    pub source: DataSource,
    pub timestamp: u64,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct PersonalBaseline {
    pub steps_14d_ma: u32,
    pub sleep_14d_ma: f32,
    pub cals_14d_ma: u16,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct DailyGoals {
    pub steps: u32,
    pub sleep_hours: f32,
    pub cals: u16,
}

#[must_use]
pub fn compute_goals(baseline: &PersonalBaseline) -> DailyGoals {
    let sleep = if baseline.sleep_14d_ma.is_finite() {
        baseline.sleep_14d_ma
    } else {
        0.0
    };
    DailyGoals {
        steps: ((baseline.steps_14d_ma as f32 * 1.15_f32).round() as u32).clamp(2_500, 18_000),
        sleep_hours: (sleep * 1.10_f32).clamp(6.0, 9.0),
        cals: ((f32::from(baseline.cals_14d_ma) * 1.15_f32).round() as u16).clamp(200, 800),
    }
}

#[must_use]
pub fn synergy_multiplier(streak_days: u32) -> f32 {
    if streak_days < 7 {
        1.0
    } else {
        (1.0_f32 + streak_days.saturating_sub(7) as f32 * 0.02_f32).clamp(1.0, 1.5)
    }
}

#[must_use]
pub fn compute_vitality(
    snapshot: &DailyActivitySnapshot,
    goals: &DailyGoals,
    streak_days: u32,
) -> u16 {
    let steps_norm = safe_ratio(snapshot.steps as f32, goals.steps as f32).clamp(0.0, 1.5);
    let sleep_hours = f32::from(snapshot.sleep_minutes) / 60.0_f32;
    let sleep_norm = safe_ratio(sleep_hours, goals.sleep_hours).clamp(0.0, 1.3);
    let cals_norm =
        safe_ratio(f32::from(snapshot.active_calories), f32::from(goals.cals)).clamp(0.0, 1.5);
    let workout_bonus = f32::from(snapshot.workout_count.min(MAX_WORKOUTS_FOR_GAIN)) * 0.3_f32;

    let base = 100.0_f32
        * (0.40_f32 * steps_norm
            + 0.25_f32 * cals_norm
            + 0.20_f32 * sleep_norm
            + 0.15_f32 * workout_bonus);
    (base * synergy_multiplier(streak_days))
        .clamp(0.0, f32::from(MAX_VITALITY_PER_DAY))
        .floor() as u16
}

fn safe_ratio(numerator: f32, denominator: f32) -> f32 {
    if numerator.is_finite() && denominator.is_finite() && denominator > 0.0 {
        numerator / denominator
    } else {
        0.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn computes_adaptive_goals() {
        let goals = compute_goals(&PersonalBaseline {
            steps_14d_ma: 8_000,
            sleep_14d_ma: 7.0,
            cals_14d_ma: 400,
        });
        assert_eq!(goals.steps, 9_200);
        assert!((goals.sleep_hours - 7.7).abs() < 0.000_01);
        assert_eq!(goals.cals, 460);
    }

    #[test]
    fn vitality_clamps_after_streak_multiplier() {
        let goals = DailyGoals {
            steps: 10_000,
            sleep_hours: 8.0,
            cals: 500,
        };
        let snapshot = DailyActivitySnapshot {
            steps: 15_000,
            sleep_minutes: 624,
            active_calories: 750,
            workout_count: 3,
            ..DailyActivitySnapshot::default()
        };
        assert_eq!(compute_vitality(&snapshot, &goals, 32), 150);
    }

    #[test]
    fn invalid_goals_fail_closed() {
        let snapshot = DailyActivitySnapshot {
            steps: 10_000,
            sleep_minutes: 480,
            active_calories: 500,
            ..DailyActivitySnapshot::default()
        };
        assert_eq!(compute_vitality(&snapshot, &DailyGoals::default(), 0), 0);
    }
}
