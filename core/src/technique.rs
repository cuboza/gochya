use serde::{Deserialize, Serialize};

use crate::{
    genome::Element,
    heart::HeartRateEvidence,
    rng::{rng_new, rng_range},
};

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum Rarity {
    #[default]
    Common = 0,
    Uncommon = 1,
    Rare = 2,
    Epic = 3,
    Legendary = 4,
    Mythic = 5,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum TechniqueType {
    #[default]
    Jab = 0,
    Hook = 1,
    Uppercut = 2,
    Cross = 3,
    Kick = 4,
    Elbow = 5,
    Block = 6,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum EffectKind {
    #[default]
    None = 0,
    Stun = 1,
    Bleed = 2,
    Crit = 3,
    Slow = 4,
    Heal = 5,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct Effect {
    pub kind: EffectKind,
    pub value: f32,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct PunchMetrics {
    pub peak_accel: f32,
    pub exec_time: f32,
    pub precision: f32,
    pub combo_len: u8,
    pub rhythm_score: f32,
    pub technique_type: TechniqueType,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct TechniqueCard {
    pub id: [u8; 16],
    pub type_: TechniqueType,
    pub element: Element,
    pub rarity: Rarity,
    pub base_damage: f32,
    pub speed: f32,
    pub stamina_cost: u16,
    /// Persisted because raw Dojo metrics never leave the device.
    pub crit_chance: f32,
    pub effect: Effect,
    pub quality: u8,
    pub owner_id: [u8; 16],
    pub created_at: u64,
}

/// Server-authoritative card fields derived from privacy-safe Dojo metrics.
#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct TechniqueStats {
    pub type_: TechniqueType,
    pub rarity: Rarity,
    pub base_damage: f32,
    pub speed: f32,
    pub stamina_cost: u16,
    pub crit_chance: f32,
    pub quality: u8,
}

#[must_use]
pub fn norm_power(peak_accel_mps2: f32) -> f32 {
    finite_or_zero((peak_accel_mps2 - 20.0_f32) / 90.0_f32).clamp(0.0, 1.0)
}

#[must_use]
pub fn combo_score(combo_len: u8) -> f32 {
    (f32::from(combo_len) / 5.0_f32).clamp(0.0, 1.0)
}

#[must_use]
pub fn crit_chance(metrics: &PunchMetrics) -> f32 {
    let rhythm = unit_interval(metrics.rhythm_score);
    (0.02_f32 + 0.01_f32 * f32::from(metrics.combo_len) + 0.05_f32 * (rhythm - 0.5_f32))
        .clamp(0.0, 0.35)
}

#[must_use]
pub const fn type_multiplier(technique_type: TechniqueType) -> f32 {
    match technique_type {
        TechniqueType::Jab => 0.9,
        TechniqueType::Hook => 1.0,
        TechniqueType::Uppercut => 1.15,
        TechniqueType::Cross | TechniqueType::Elbow => 1.1,
        TechniqueType::Kick => 1.2,
        TechniqueType::Block => 0.3,
    }
}

#[must_use]
pub fn derive_technique_stats(
    metrics: &PunchMetrics,
    heart: &HeartRateEvidence,
    tech_level: f32,
) -> TechniqueStats {
    let peak_accel = finite_or_zero(metrics.peak_accel).max(0.0);
    let execution_time = finite_or_zero(metrics.exec_time).max(0.0);
    let level = if tech_level.is_finite() {
        tech_level.clamp(1.0, 1.5)
    } else {
        1.0
    };
    let quality = quality_score(metrics, heart);
    let power_rating = peak_accel / 50.0_f32;

    TechniqueStats {
        type_: metrics.technique_type,
        rarity: rarity_from_quality(quality),
        base_damage: power_rating
            * unit_interval(metrics.precision)
            * type_multiplier(metrics.technique_type)
            * level
            * 100.0_f32,
        speed: 100.0_f32 / (1.0_f32 + execution_time),
        stamina_cost: (power_rating * 2.2_f32)
            .round()
            .clamp(0.0, f32::from(u16::MAX)) as u16,
        crit_chance: crit_chance(metrics),
        quality,
    }
}

/// Generates a server-authoritative Technique Card reward from an explicit seed.
///
/// The caller selects the source-specific rarity ceiling. Game-loot sources are
/// intentionally limited to Epic or below; Legendary and Mythic remain Dojo
/// and gacha territory.
#[must_use]
pub fn generate_loot_technique_stats(seed: u64, max_rarity: Rarity) -> TechniqueStats {
    let rarity_cap = (max_rarity as u8).min(Rarity::Epic as u8);
    let mut rng = rng_new(seed);
    let rarity = loot_rarity(rng_range(&mut rng, 0, 99), rarity_cap);
    // Effect-less Block cards are not emitted until loot effects are part of
    // the persisted card contract.
    let type_ = technique_type_from_roll(rng_range(&mut rng, 0, 5));
    let (quality_min, quality_max) = rarity_quality_bounds(rarity);
    let quality = rng_range(&mut rng, u32::from(quality_min), u32::from(quality_max)) as u8;
    let speed_jitter = rng_range(&mut rng, 0, 10) as f32 - 5.0_f32;
    let crit_jitter = rng_range(&mut rng, 0, 30) as f32 / 1_000.0_f32;

    TechniqueStats {
        type_,
        rarity,
        base_damage: (80.0_f32 + f32::from(quality)) * type_multiplier(type_),
        speed: (technique_base_speed(type_) + speed_jitter).max(1.0),
        stamina_cost: technique_stamina_cost(type_),
        crit_chance: (0.03_f32 + f32::from(quality) / 1_000.0_f32 + crit_jitter).clamp(0.0, 0.35),
        quality,
    }
}

const fn loot_rarity(roll: u32, max_rarity: u8) -> Rarity {
    match max_rarity {
        0 => Rarity::Common,
        1 => {
            if roll < 70 {
                Rarity::Common
            } else {
                Rarity::Uncommon
            }
        }
        2 => {
            if roll < 60 {
                Rarity::Common
            } else if roll < 90 {
                Rarity::Uncommon
            } else {
                Rarity::Rare
            }
        }
        _ => {
            if roll < 50 {
                Rarity::Common
            } else if roll < 80 {
                Rarity::Uncommon
            } else if roll < 95 {
                Rarity::Rare
            } else {
                Rarity::Epic
            }
        }
    }
}

const fn rarity_quality_bounds(rarity: Rarity) -> (u8, u8) {
    match rarity {
        Rarity::Common => (30, 39),
        Rarity::Uncommon => (40, 54),
        Rarity::Rare => (55, 69),
        Rarity::Epic => (70, 84),
        Rarity::Legendary => (85, 94),
        Rarity::Mythic => (95, 100),
    }
}

const fn technique_type_from_roll(roll: u32) -> TechniqueType {
    match roll {
        0 => TechniqueType::Jab,
        1 => TechniqueType::Hook,
        2 => TechniqueType::Uppercut,
        3 => TechniqueType::Cross,
        4 => TechniqueType::Kick,
        _ => TechniqueType::Elbow,
    }
}

const fn technique_base_speed(technique_type: TechniqueType) -> f32 {
    match technique_type {
        TechniqueType::Jab => 82.0,
        TechniqueType::Hook => 66.0,
        TechniqueType::Uppercut => 56.0,
        TechniqueType::Cross => 62.0,
        TechniqueType::Kick => 50.0,
        TechniqueType::Elbow => 72.0,
        TechniqueType::Block => 76.0,
    }
}

const fn technique_stamina_cost(technique_type: TechniqueType) -> u16 {
    match technique_type {
        TechniqueType::Jab => 6,
        TechniqueType::Hook => 9,
        TechniqueType::Uppercut => 12,
        TechniqueType::Cross => 11,
        TechniqueType::Kick => 14,
        TechniqueType::Elbow => 10,
        TechniqueType::Block => 7,
    }
}

#[must_use]
pub fn quality_score(metrics: &PunchMetrics, heart: &HeartRateEvidence) -> u8 {
    let weighted = 0.40_f32 * norm_power(metrics.peak_accel)
        + 0.25_f32 * unit_interval(metrics.precision)
        + 0.12_f32 * combo_score(metrics.combo_len)
        + 0.08_f32 * unit_interval(metrics.rhythm_score)
        + 0.15_f32 * crate::heart::heart_score(heart);
    (100.0_f32 * weighted).round().clamp(0.0, 100.0) as u8
}

#[must_use]
pub const fn rarity_from_quality(quality: u8) -> Rarity {
    match quality {
        0..=39 => Rarity::Common,
        40..=54 => Rarity::Uncommon,
        55..=69 => Rarity::Rare,
        70..=84 => Rarity::Epic,
        85..=94 => Rarity::Legendary,
        _ => Rarity::Mythic,
    }
}

#[must_use]
pub fn muscle_memory_bonus(repeat_count_of_type: u32) -> f32 {
    (repeat_count_of_type as f32 / 50.0_f32 * 0.01_f32).clamp(0.0, 0.15)
}

#[must_use]
pub fn tech_card_bonus(card_type: TechniqueType, affinity: TechniqueType) -> f32 {
    if card_type == affinity { 0.15 } else { 0.0 }
}

pub(crate) fn finite_or_zero(value: f32) -> f32 {
    if value.is_finite() { value } else { 0.0 }
}

pub(crate) fn unit_interval(value: f32) -> f32 {
    finite_or_zero(value).clamp(0.0, 1.0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::heart::HeartRateEvidence;

    fn heart() -> HeartRateEvidence {
        HeartRateEvidence {
            baseline: 70,
            mean: 90,
            present: 0.9,
            confidence: 0.9,
            delta: 20,
        }
    }

    #[test]
    fn quality_uses_all_terms_in_declared_order() {
        let metrics = PunchMetrics {
            peak_accel: 65.0,
            exec_time: 0.5,
            precision: 0.8,
            combo_len: 3,
            rhythm_score: 0.75,
            technique_type: TechniqueType::Hook,
        };
        assert_eq!(quality_score(&metrics, &heart()), 64);
    }

    #[test]
    fn invalid_floats_fail_to_safe_values() {
        let metrics = PunchMetrics {
            peak_accel: f32::NAN,
            precision: f32::INFINITY,
            rhythm_score: f32::NEG_INFINITY,
            ..PunchMetrics::default()
        };
        assert_eq!(quality_score(&metrics, &heart()), 11);
    }

    #[test]
    fn rarity_boundaries_match_contract() {
        assert_eq!(rarity_from_quality(39), Rarity::Common);
        assert_eq!(rarity_from_quality(40), Rarity::Uncommon);
        assert_eq!(rarity_from_quality(95), Rarity::Mythic);
    }

    #[test]
    fn derives_all_server_card_stats_without_raw_signal() {
        let metrics = PunchMetrics {
            peak_accel: 65.0,
            exec_time: 0.5,
            precision: 0.8,
            combo_len: 3,
            rhythm_score: 0.75,
            technique_type: TechniqueType::Hook,
        };
        let stats = derive_technique_stats(&metrics, &heart(), 1.0);
        assert_eq!(stats.type_, TechniqueType::Hook);
        assert_eq!(stats.rarity, Rarity::Rare);
        assert!((stats.base_damage - 104.0).abs() < 0.000_01);
        assert!((stats.speed - 66.666_664).abs() < 0.000_01);
        assert_eq!(stats.stamina_cost, 3);
        assert!((stats.crit_chance - 0.0625).abs() < 0.000_01);
        assert_eq!(stats.quality, 64);
    }

    #[test]
    fn game_loot_is_deterministic_and_respects_rare_cap() {
        let stats = generate_loot_technique_stats(42, Rarity::Rare);
        assert_eq!(stats, generate_loot_technique_stats(42, Rarity::Rare));
        assert!((stats.rarity as u8) <= Rarity::Rare as u8);
        assert!((0..=5).contains(&(stats.type_ as u8)));
        assert_eq!(rarity_from_quality(stats.quality), stats.rarity);
        assert_eq!(stats.type_, TechniqueType::Elbow);
        assert_eq!(stats.rarity, Rarity::Common);
        assert_eq!(stats.quality, 35);
        assert_eq!(stats.base_damage, 126.5);
        assert!((0.0..=0.35).contains(&stats.crit_chance));
    }

    #[test]
    fn game_loot_never_exceeds_epic() {
        for seed in 0..10_000 {
            let stats = generate_loot_technique_stats(seed, Rarity::Mythic);
            assert!((stats.rarity as u8) <= Rarity::Epic as u8);
        }
    }
}
