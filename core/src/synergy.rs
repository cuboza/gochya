use serde::{Deserialize, Serialize};

use crate::genome::{Element, Genome};

pub const MAX_WORKOUTS: usize = 8;
pub const MAX_WORKOUTS_FOR_GAIN: u8 = 3;
pub const MAX_VITALITY_PER_DAY: u16 = 150;

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum WorkoutKind {
    #[default]
    Running = 0,
    Cycling = 1,
    Strength = 2,
    Swimming = 3,
    Yoga = 4,
    Meditation = 5,
    Hiit = 6,
    Other = 255,
}

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

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct StatGains {
    pub str: i16,
    pub agi: i16,
    pub end: i16,
    pub foc: i16,
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

#[must_use]
pub fn compute_stat_gains(
    snapshot: &DailyActivitySnapshot,
    _goals: &DailyGoals,
    genome: &Genome,
    streak_days: u32,
) -> StatGains {
    let workout_count =
        usize::from(snapshot.workout_count.min(MAX_WORKOUTS_FOR_GAIN)).min(MAX_WORKOUTS);
    let mut strength_minutes = 0.0_f32;
    let mut cardio_minutes = 0.0_f32;
    let mut total_minutes = 0.0_f32;
    for workout in snapshot.workouts.iter().take(workout_count) {
        let weighted_minutes = f32::from(workout.duration_min)
            * (1.0_f32 + resonance_bonus(workout.kind, genome.element));
        total_minutes += weighted_minutes;
        if workout.kind == WorkoutKind::Strength as u8 {
            strength_minutes += weighted_minutes;
        }
        if matches!(
            workout.kind,
            kind if kind == WorkoutKind::Running as u8
                || kind == WorkoutKind::Cycling as u8
                || kind == WorkoutKind::Swimming as u8
                || kind == WorkoutKind::Hiit as u8
        ) {
            cardio_minutes += weighted_minutes;
        }
    }

    let strength = strength_minutes / 30.0_f32 * 5.0_f32 + f32::from(snapshot.floors) / 10.0 * 2.0;
    let agility =
        cardio_minutes / 30.0_f32 * 5.0_f32 + f32::from(snapshot.hr_zone_high_min) / 10.0 * 2.0;
    let streak_bonus = (streak_days as f32 * 0.2_f32).min(3.0);
    let endurance = total_minutes / 60.0_f32 * 5.0_f32 + streak_bonus;
    let focus = f32::from(snapshot.meditation_min) / 15.0_f32 * 3.0_f32
        + f32::from(snapshot.sleep_quality.min(100)) / 100.0_f32 * 5.0_f32
        - f32::from(snapshot.stress_level.min(100)) / 20.0_f32;

    StatGains {
        str: floor_to_i16(strength),
        agi: floor_to_i16(agility),
        end: floor_to_i16(endurance),
        foc: floor_to_i16(focus),
    }
}

#[must_use]
pub fn resonance_bonus(workout_kind: u8, element: Element) -> f32 {
    let resonates = matches!(
        (workout_kind, element),
        (kind, Element::Fire) if kind == WorkoutKind::Running as u8
    ) || matches!(
        (workout_kind, element),
        (kind, Element::Air) if kind == WorkoutKind::Cycling as u8
    ) || matches!(
        (workout_kind, element),
        (kind, Element::Earth) if kind == WorkoutKind::Strength as u8
    ) || matches!(
        (workout_kind, element),
        (kind, Element::Water) if kind == WorkoutKind::Swimming as u8
    ) || matches!(
        (workout_kind, element),
        (kind, Element::Light) if kind == WorkoutKind::Yoga as u8
    ) || matches!(
        (workout_kind, element),
        (kind, Element::Arcane) if kind == WorkoutKind::Meditation as u8
    ) || matches!(
        (workout_kind, element),
        (kind, Element::Dark) if kind == WorkoutKind::Hiit as u8
    );
    if resonates { 0.10 } else { 0.0 }
}

fn safe_ratio(numerator: f32, denominator: f32) -> f32 {
    if numerator.is_finite() && denominator.is_finite() && denominator > 0.0 {
        numerator / denominator
    } else {
        0.0
    }
}

fn floor_to_i16(value: f32) -> i16 {
    value
        .floor()
        .clamp(f32::from(i16::MIN), f32::from(i16::MAX)) as i16
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::genome::{StatPotentials, VisualGenes};
    use crate::technique::{Rarity, TechniqueType};

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

    #[test]
    fn computes_stat_gains_from_first_three_workouts() {
        let snapshot = DailyActivitySnapshot {
            workouts: [
                workout(WorkoutKind::Strength, 30),
                workout(WorkoutKind::Running, 30),
                workout(WorkoutKind::Yoga, 60),
                workout(WorkoutKind::Strength, 600),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
            ],
            workout_count: 4,
            sleep_quality: 100,
            hr_zone_high_min: 10,
            meditation_min: 15,
            stress_level: 20,
            floors: 10,
            ..DailyActivitySnapshot::default()
        };
        assert_eq!(
            compute_stat_gains(
                &snapshot,
                &DailyGoals::default(),
                &genome(Element::Earth),
                10,
            ),
            StatGains {
                str: 7,
                agi: 7,
                end: 12,
                foc: 7,
            }
        );
    }

    #[test]
    fn resonance_weights_matching_workout_minutes() {
        let snapshot = DailyActivitySnapshot {
            workouts: [
                workout(WorkoutKind::Running, 55),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
                WorkoutSummary::default(),
            ],
            workout_count: 1,
            ..DailyActivitySnapshot::default()
        };
        let fire = compute_stat_gains(&snapshot, &DailyGoals::default(), &genome(Element::Fire), 0);
        let water = compute_stat_gains(
            &snapshot,
            &DailyGoals::default(),
            &genome(Element::Water),
            0,
        );
        assert_eq!(fire.agi, 10);
        assert_eq!(water.agi, 9);
    }

    #[test]
    fn focus_can_be_negative_and_extreme_inputs_saturate() {
        let snapshot = DailyActivitySnapshot {
            workouts: [WorkoutSummary {
                kind: WorkoutKind::Strength as u8,
                duration_min: u16::MAX,
                calories: 0,
            }; MAX_WORKOUTS],
            workout_count: u8::MAX,
            stress_level: 100,
            ..DailyActivitySnapshot::default()
        };
        let gains = compute_stat_gains(
            &snapshot,
            &DailyGoals::default(),
            &genome(Element::Earth),
            u32::MAX,
        );
        assert_eq!(gains.str, i16::MAX);
        assert_eq!(gains.foc, -5);
    }

    #[test]
    fn resonance_table_matches_balance_contract() {
        for (kind, element) in [
            (WorkoutKind::Running, Element::Fire),
            (WorkoutKind::Cycling, Element::Air),
            (WorkoutKind::Strength, Element::Earth),
            (WorkoutKind::Swimming, Element::Water),
            (WorkoutKind::Yoga, Element::Light),
            (WorkoutKind::Meditation, Element::Arcane),
            (WorkoutKind::Hiit, Element::Dark),
        ] {
            assert_eq!(resonance_bonus(kind as u8, element), 0.10);
        }
        assert_eq!(
            resonance_bonus(WorkoutKind::Other as u8, Element::Fire),
            0.0
        );
        assert_eq!(resonance_bonus(u8::MAX - 1, Element::Fire), 0.0);
    }

    fn workout(kind: WorkoutKind, duration_min: u16) -> WorkoutSummary {
        WorkoutSummary {
            kind: kind as u8,
            duration_min,
            calories: 0,
        }
    }

    fn genome(element: Element) -> Genome {
        Genome {
            visual: VisualGenes::default(),
            stats: StatPotentials::default(),
            element,
            tech_affinity: TechniqueType::default(),
            rarity: Rarity::default(),
            ability: crate::genome::Ability::default(),
            generation: 0,
        }
    }
}
