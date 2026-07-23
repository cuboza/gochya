use gochya_core::{
    DailyActivitySnapshot, DailyGoals, Element, Genome, HeartRateEvidence, Loadout, Match,
    MatchMode, PunchMetrics, Stats, TechniqueCard, TechniqueType, compute_vitality, quality_score,
    simulate_combat,
};
use proptest::prelude::*;

proptest! {
    #[test]
    fn quality_is_always_bounded(
        peak_accel in any::<f32>(),
        precision in any::<f32>(),
        combo_len in any::<u8>(),
        rhythm_score in any::<f32>(),
        present in any::<f32>(),
        confidence in any::<f32>(),
        delta in any::<i16>(),
    ) {
        let metrics = PunchMetrics {
            peak_accel,
            precision,
            combo_len,
            rhythm_score,
            ..PunchMetrics::default()
        };
        let heart = HeartRateEvidence {
            baseline: 70,
            mean: 90,
            present,
            confidence,
            delta,
        };
        prop_assert!(quality_score(&metrics, &heart) <= 100);
    }

    #[test]
    fn vitality_is_always_bounded(
        steps in any::<u32>(),
        sleep_minutes in any::<u16>(),
        active_calories in any::<u16>(),
        workout_count in any::<u8>(),
        goal_steps in any::<u32>(),
        goal_sleep in any::<f32>(),
        goal_cals in any::<u16>(),
        streak_days in any::<u32>(),
    ) {
        let snapshot = DailyActivitySnapshot {
            steps,
            sleep_minutes,
            active_calories,
            workout_count,
            ..DailyActivitySnapshot::default()
        };
        let goals = DailyGoals {
            steps: goal_steps,
            sleep_hours: goal_sleep,
            cals: goal_cals,
        };
        prop_assert!(compute_vitality(&snapshot, &goals, streak_days) <= 150);
    }

    #[test]
    fn combat_is_deterministic(seed in any::<u64>(), damage_a in 1.0_f32..300.0, damage_b in 1.0_f32..300.0) {
        let match_ = match_fixture(damage_a, damage_b);
        prop_assert_eq!(simulate_combat(&match_, seed), simulate_combat(&match_, seed));
    }
}

fn match_fixture(damage_a: f32, damage_b: f32) -> Match {
    Match {
        loadout_a: loadout(Element::Fire, damage_a, 70.0),
        loadout_b: loadout(Element::Earth, damage_b, 60.0),
        mode: MatchMode::Casual,
    }
}

fn loadout(element: Element, damage: f32, speed: f32) -> Loadout {
    let card = TechniqueCard {
        type_: TechniqueType::Jab,
        element,
        base_damage: damage,
        speed,
        stamina_cost: 10,
        quality: 50,
        ..TechniqueCard::default()
    };
    Loadout {
        pet_stats: Stats {
            str: 30,
            agi: 30,
            end: 30,
            foc: 30,
        },
        pet_genome: Genome {
            element,
            tech_affinity: TechniqueType::Jab,
            ..Genome::default()
        },
        pet_mood: 100,
        cards: [card; 5],
        signature_idx: 4,
        ..Loadout::default()
    }
}
