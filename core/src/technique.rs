use serde::{Deserialize, Serialize};

use crate::{genome::Element, heart::HeartRateEvidence};

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
            * level,
        speed: 100.0_f32 / (1.0_f32 + execution_time),
        stamina_cost: (power_rating * 2.2_f32)
            .round()
            .clamp(0.0, f32::from(u16::MAX)) as u16,
        crit_chance: crit_chance(metrics),
        quality,
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
        assert!((stats.base_damage - 1.04).abs() < 0.000_01);
        assert!((stats.speed - 66.666_664).abs() < 0.000_01);
        assert_eq!(stats.stamina_cost, 3);
        assert!((stats.crit_chance - 0.0625).abs() < 0.000_01);
        assert_eq!(stats.quality, 64);
    }
}
