//! Stable C ABI. Domain structs deliberately do not cross this boundary.
#![allow(clippy::not_unsafe_ptr_arg_deref)]

use std::{mem::size_of, panic};

use crate::{
    HeartRateEvidence, PunchMetrics,
    combat::{GearSummary, Loadout, Match, MatchMode, simulate_combat},
    genome::{Element, Genome},
    heart::validate_heart,
    pet::Stats,
    synergy::{DailyActivitySnapshot, DailyGoals, compute_vitality},
    technique::{
        Effect, EffectKind, TechniqueCard, TechniqueType, derive_technique_stats, quality_score,
    },
};

pub const ABI_VERSION: u32 = 0x0001_0100;
pub const ABI_SCHEMA_VERSION: u16 = 1;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(i32)]
pub enum GochyaStatus {
    Ok = 0,
    InvalidArgument = 1,
    BufferTooSmall = 2,
    SchemaMismatch = 3,
    DomainRejected = 4,
    InternalError = 255,
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaPunchMetricsV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub technique_type: u8,
    pub combo_len: u8,
    pub peak_accel_mps2: f32,
    pub exec_time_seconds: f32,
    pub precision: f32,
    pub rhythm_score: f32,
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaHeartEvidenceV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub reserved0: [u8; 2],
    pub baseline_bpm: u16,
    pub mean_bpm: u16,
    pub present: f32,
    pub confidence: f32,
    pub delta_bpm: i16,
    pub reserved: [u8; 14],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaHeartVerdictV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub passed: u8,
    pub reason: u8,
    pub heart_score: f32,
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaDailyActivityV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub workout_count: u8,
    pub reserved0: u8,
    pub steps: u32,
    pub sleep_minutes: u16,
    pub active_calories: u16,
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaDailyGoalsV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub reserved0: [u8; 2],
    pub steps: u32,
    pub sleep_hours: f32,
    pub active_calories: u16,
    pub reserved: [u8; 14],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaTechniqueStatsV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub technique_type: u8,
    pub rarity: u8,
    pub base_damage: f32,
    pub speed: f32,
    pub crit_chance: f32,
    pub stamina_cost: u16,
    pub quality: u8,
    pub reserved0: u8,
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaCombatCardV1 {
    pub base_damage: f32,
    pub speed: f32,
    pub crit_chance: f32,
    pub effect_value: f32,
    pub stamina_cost: u16,
    pub technique_type: u8,
    pub effect_kind: u8,
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaCombatLoadoutV1 {
    pub stat_str: u32,
    pub stat_agi: u32,
    pub stat_end: u32,
    pub stat_foc: u32,
    pub gear_str_bonus: i16,
    pub gear_agi_bonus: i16,
    pub gear_end_bonus: i16,
    pub gear_foc_bonus: i16,
    pub element: u8,
    pub tech_affinity: u8,
    pub pet_mood: u8,
    pub signature_idx: u8,
    pub cards: [GochyaCombatCardV1; 5],
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaCombatMatchV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub mode: u8,
    pub reserved0: u8,
    pub loadout_a: GochyaCombatLoadoutV1,
    pub loadout_b: GochyaCombatLoadoutV1,
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaCombatRoundV1 {
    pub card_a_idx: u8,
    pub card_b_idx: u8,
    pub effect_kind: u8,
    pub reserved0: u8,
    pub damage_a_to_b: u16,
    pub damage_b_to_a: u16,
    pub effect_value: f32,
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaCombatResultV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub winner: u8,
    pub round_count: u8,
    pub final_hp_a: u16,
    pub final_hp_b: u16,
    pub seed: u64,
    pub rounds: [GochyaCombatRoundV1; 20],
    pub reserved: [u8; 16],
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_abi_version() -> u32 {
    ABI_VERSION
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_quality_score_v1(
    metrics: *const GochyaPunchMetricsV1,
    heart: *const GochyaHeartEvidenceV1,
    out_score: *mut u8,
) -> GochyaStatus {
    catch_status(|| {
        if metrics.is_null() || heart.is_null() || out_score.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable V1 inputs and writable output.
        let (metrics, heart) = unsafe { (&*metrics, &*heart) };
        if !valid_header(
            metrics.struct_size,
            metrics.schema_version,
            size_of::<GochyaPunchMetricsV1>(),
        ) || !valid_header(
            heart.struct_size,
            heart.schema_version,
            size_of::<GochyaHeartEvidenceV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        let Some(technique_type) = technique_type_from_u8(metrics.technique_type) else {
            return GochyaStatus::InvalidArgument;
        };
        if !all_finite(&[
            metrics.peak_accel_mps2,
            metrics.exec_time_seconds,
            metrics.precision,
            metrics.rhythm_score,
            heart.present,
            heart.confidence,
        ]) {
            return GochyaStatus::InvalidArgument;
        }
        let domain_metrics = PunchMetrics {
            peak_accel: metrics.peak_accel_mps2,
            exec_time: metrics.exec_time_seconds,
            precision: metrics.precision,
            combo_len: metrics.combo_len,
            rhythm_score: metrics.rhythm_score,
            technique_type,
        };
        let domain_heart = heart_from_ffi(heart);
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe { *out_score = quality_score(&domain_metrics, &domain_heart) };
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_validate_heart_v1(
    heart: *const GochyaHeartEvidenceV1,
    out_verdict: *mut GochyaHeartVerdictV1,
) -> GochyaStatus {
    catch_status(|| {
        if heart.is_null() || out_verdict.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable/writable memory.
        let heart = unsafe { &*heart };
        if !valid_header(
            heart.struct_size,
            heart.schema_version,
            size_of::<GochyaHeartEvidenceV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        if !all_finite(&[heart.present, heart.confidence]) {
            return GochyaStatus::InvalidArgument;
        }
        let verdict = validate_heart(&heart_from_ffi(heart));
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe {
            *out_verdict = GochyaHeartVerdictV1 {
                struct_size: size_u32::<GochyaHeartVerdictV1>(),
                schema_version: ABI_SCHEMA_VERSION,
                passed: u8::from(verdict.passed),
                reason: verdict.reason as u8,
                heart_score: verdict.heart_score,
                reserved: [0; 16],
            };
        }
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_compute_vitality_v1(
    activity: *const GochyaDailyActivityV1,
    goals: *const GochyaDailyGoalsV1,
    streak_days: u32,
    out_vitality: *mut u16,
) -> GochyaStatus {
    catch_status(|| {
        if activity.is_null() || goals.is_null() || out_vitality.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable V1 inputs.
        let (activity, goals) = unsafe { (&*activity, &*goals) };
        if !valid_header(
            activity.struct_size,
            activity.schema_version,
            size_of::<GochyaDailyActivityV1>(),
        ) || !valid_header(
            goals.struct_size,
            goals.schema_version,
            size_of::<GochyaDailyGoalsV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        if !goals.sleep_hours.is_finite() {
            return GochyaStatus::InvalidArgument;
        }
        let snapshot = DailyActivitySnapshot {
            steps: activity.steps,
            sleep_minutes: activity.sleep_minutes,
            active_calories: activity.active_calories,
            workout_count: activity.workout_count,
            ..DailyActivitySnapshot::default()
        };
        let domain_goals = DailyGoals {
            steps: goals.steps,
            sleep_hours: goals.sleep_hours,
            cals: goals.active_calories,
        };
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe { *out_vitality = compute_vitality(&snapshot, &domain_goals, streak_days) };
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_derive_technique_v1(
    metrics: *const GochyaPunchMetricsV1,
    heart: *const GochyaHeartEvidenceV1,
    tech_level: f32,
    out_stats: *mut GochyaTechniqueStatsV1,
) -> GochyaStatus {
    catch_status(|| {
        if metrics.is_null() || heart.is_null() || out_stats.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable V1 inputs.
        let (metrics, heart) = unsafe { (&*metrics, &*heart) };
        if !valid_header(
            metrics.struct_size,
            metrics.schema_version,
            size_of::<GochyaPunchMetricsV1>(),
        ) || !valid_header(
            heart.struct_size,
            heart.schema_version,
            size_of::<GochyaHeartEvidenceV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        let Some(technique_type) = technique_type_from_u8(metrics.technique_type) else {
            return GochyaStatus::InvalidArgument;
        };
        if !all_finite(&[
            metrics.peak_accel_mps2,
            metrics.exec_time_seconds,
            metrics.precision,
            metrics.rhythm_score,
            heart.present,
            heart.confidence,
            tech_level,
        ]) {
            return GochyaStatus::InvalidArgument;
        }
        let domain_metrics = PunchMetrics {
            peak_accel: metrics.peak_accel_mps2,
            exec_time: metrics.exec_time_seconds,
            precision: metrics.precision,
            combo_len: metrics.combo_len,
            rhythm_score: metrics.rhythm_score,
            technique_type,
        };
        let stats = derive_technique_stats(&domain_metrics, &heart_from_ffi(heart), tech_level);
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe {
            *out_stats = GochyaTechniqueStatsV1 {
                struct_size: size_u32::<GochyaTechniqueStatsV1>(),
                schema_version: ABI_SCHEMA_VERSION,
                technique_type: stats.type_ as u8,
                rarity: stats.rarity as u8,
                base_damage: stats.base_damage,
                speed: stats.speed,
                crit_chance: stats.crit_chance,
                stamina_cost: stats.stamina_cost,
                quality: stats.quality,
                reserved0: 0,
                reserved: [0; 16],
            };
        }
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_simulate_combat_v1(
    match_: *const GochyaCombatMatchV1,
    seed: u64,
    out_result: *mut GochyaCombatResultV1,
) -> GochyaStatus {
    catch_status(|| {
        if match_.is_null() || out_result.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable/writable memory.
        let input = unsafe { &*match_ };
        if !valid_header(
            input.struct_size,
            input.schema_version,
            size_of::<GochyaCombatMatchV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        let Some(mode) = match_mode_from_u8(input.mode) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(loadout_a) = combat_loadout_from_ffi(&input.loadout_a) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(loadout_b) = combat_loadout_from_ffi(&input.loadout_b) else {
            return GochyaStatus::InvalidArgument;
        };
        let result = simulate_combat(
            &Match {
                loadout_a,
                loadout_b,
                mode,
            },
            seed,
        );
        let mut rounds = [GochyaCombatRoundV1::default(); 20];
        for (output, round) in rounds.iter_mut().zip(result.rounds) {
            *output = GochyaCombatRoundV1 {
                card_a_idx: round.card_a_idx,
                card_b_idx: round.card_b_idx,
                effect_kind: round.effect_triggered.kind as u8,
                reserved0: 0,
                damage_a_to_b: round.damage_a_to_b,
                damage_b_to_a: round.damage_b_to_a,
                effect_value: round.effect_triggered.value,
            };
        }
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe {
            *out_result = GochyaCombatResultV1 {
                struct_size: size_u32::<GochyaCombatResultV1>(),
                schema_version: ABI_SCHEMA_VERSION,
                winner: result.winner as u8,
                round_count: result.round_count,
                final_hp_a: result.final_hp_a,
                final_hp_b: result.final_hp_b,
                seed: result.seed,
                rounds,
                reserved: [0; 16],
            };
        }
        GochyaStatus::Ok
    })
}

fn combat_loadout_from_ffi(input: &GochyaCombatLoadoutV1) -> Option<Loadout> {
    if input.pet_mood > 100 || input.signature_idx > 4 {
        return None;
    }
    let element = element_from_u8(input.element)?;
    let tech_affinity = technique_type_from_u8(input.tech_affinity)?;
    let mut cards = [TechniqueCard::default(); 5];
    for (output, card) in cards.iter_mut().zip(input.cards) {
        let technique_type = technique_type_from_u8(card.technique_type)?;
        let effect_kind = effect_kind_from_u8(card.effect_kind)?;
        if !all_finite(&[
            card.base_damage,
            card.speed,
            card.crit_chance,
            card.effect_value,
        ]) || card.base_damage < 0.0
            || card.speed < 0.0
            || !(0.0..=0.35).contains(&card.crit_chance)
            || card.effect_value < 0.0
        {
            return None;
        }
        *output = TechniqueCard {
            type_: technique_type,
            element,
            base_damage: card.base_damage,
            speed: card.speed,
            stamina_cost: card.stamina_cost,
            crit_chance: card.crit_chance,
            effect: Effect {
                kind: effect_kind,
                value: card.effect_value,
            },
            ..TechniqueCard::default()
        };
    }
    Some(Loadout {
        pet_stats: Stats {
            str: input.stat_str,
            agi: input.stat_agi,
            end: input.stat_end,
            foc: input.stat_foc,
        },
        pet_genome: Genome {
            element,
            tech_affinity,
            ..Genome::default()
        },
        pet_mood: input.pet_mood,
        cards,
        signature_idx: input.signature_idx,
        gear: GearSummary {
            str_bonus: input.gear_str_bonus,
            agi_bonus: input.gear_agi_bonus,
            end_bonus: input.gear_end_bonus,
            foc_bonus: input.gear_foc_bonus,
            element,
        },
        ..Loadout::default()
    })
}

fn heart_from_ffi(heart: &GochyaHeartEvidenceV1) -> HeartRateEvidence {
    HeartRateEvidence {
        baseline: heart.baseline_bpm,
        mean: heart.mean_bpm,
        present: heart.present,
        confidence: heart.confidence,
        delta: heart.delta_bpm,
    }
}

fn catch_status(body: impl FnOnce() -> GochyaStatus) -> GochyaStatus {
    panic::catch_unwind(panic::AssertUnwindSafe(body)).unwrap_or(GochyaStatus::InternalError)
}

fn valid_header(struct_size: u32, schema_version: u16, expected_size: usize) -> bool {
    struct_size == u32::try_from(expected_size).unwrap_or(u32::MAX)
        && schema_version == ABI_SCHEMA_VERSION
}

fn size_u32<T>() -> u32 {
    u32::try_from(size_of::<T>()).unwrap_or(u32::MAX)
}

fn all_finite(values: &[f32]) -> bool {
    values.iter().all(|value| value.is_finite())
}

const fn technique_type_from_u8(value: u8) -> Option<TechniqueType> {
    match value {
        0 => Some(TechniqueType::Jab),
        1 => Some(TechniqueType::Hook),
        2 => Some(TechniqueType::Uppercut),
        3 => Some(TechniqueType::Cross),
        4 => Some(TechniqueType::Kick),
        5 => Some(TechniqueType::Elbow),
        6 => Some(TechniqueType::Block),
        _ => None,
    }
}

const fn element_from_u8(value: u8) -> Option<Element> {
    match value {
        0 => Some(Element::Fire),
        1 => Some(Element::Water),
        2 => Some(Element::Earth),
        3 => Some(Element::Air),
        4 => Some(Element::Light),
        5 => Some(Element::Dark),
        6 => Some(Element::Arcane),
        7 => Some(Element::Steam),
        8 => Some(Element::Magma),
        9 => Some(Element::Storm),
        10 => Some(Element::Mud),
        11 => Some(Element::Smoke),
        12 => Some(Element::Sand),
        13 => Some(Element::Eclipse),
        14 => Some(Element::Inferno),
        15 => Some(Element::Prism),
        16 => Some(Element::Crystal),
        _ => None,
    }
}

const fn effect_kind_from_u8(value: u8) -> Option<EffectKind> {
    match value {
        0 => Some(EffectKind::None),
        1 => Some(EffectKind::Stun),
        2 => Some(EffectKind::Bleed),
        3 => Some(EffectKind::Crit),
        4 => Some(EffectKind::Slow),
        5 => Some(EffectKind::Heal),
        _ => None,
    }
}

const fn match_mode_from_u8(value: u8) -> Option<MatchMode> {
    match value {
        0 => Some(MatchMode::Casual),
        1 => Some(MatchMode::Ranked),
        2 => Some(MatchMode::Tournament),
        _ => None,
    }
}

const _: () = {
    assert!(size_of::<GochyaPunchMetricsV1>() == 40);
    assert!(size_of::<GochyaHeartEvidenceV1>() == 36);
    assert!(size_of::<GochyaHeartVerdictV1>() == 28);
    assert!(size_of::<GochyaDailyActivityV1>() == 32);
    assert!(size_of::<GochyaDailyGoalsV1>() == 32);
    assert!(size_of::<GochyaTechniqueStatsV1>() == 40);
    assert!(size_of::<GochyaCombatCardV1>() == 20);
    assert!(size_of::<GochyaCombatLoadoutV1>() == 144);
    assert!(size_of::<GochyaCombatMatchV1>() == 312);
    assert!(size_of::<GochyaCombatRoundV1>() == 12);
    assert!(size_of::<GochyaCombatResultV1>() == 280);
};

#[cfg(test)]
mod tests {
    use super::*;
    use crate::HeartFailReason;

    fn ffi_heart() -> GochyaHeartEvidenceV1 {
        GochyaHeartEvidenceV1 {
            struct_size: size_u32::<GochyaHeartEvidenceV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            baseline_bpm: 70,
            mean_bpm: 90,
            present: 0.9,
            confidence: 0.9,
            delta_bpm: 20,
            ..GochyaHeartEvidenceV1::default()
        }
    }

    fn ffi_combat_loadout(element: Element, base_damage: f32, speed: f32) -> GochyaCombatLoadoutV1 {
        let card = GochyaCombatCardV1 {
            base_damage,
            speed,
            stamina_cost: 10,
            technique_type: TechniqueType::Jab as u8,
            ..GochyaCombatCardV1::default()
        };
        GochyaCombatLoadoutV1 {
            stat_str: 30,
            stat_agi: 30,
            stat_end: 30,
            stat_foc: 30,
            element: element as u8,
            tech_affinity: TechniqueType::Jab as u8,
            pet_mood: 100,
            signature_idx: 4,
            cards: [card; 5],
            ..GochyaCombatLoadoutV1::default()
        }
    }

    fn ffi_combat_match() -> GochyaCombatMatchV1 {
        GochyaCombatMatchV1 {
            struct_size: size_u32::<GochyaCombatMatchV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            mode: MatchMode::Casual as u8,
            loadout_a: ffi_combat_loadout(Element::Fire, 260.0, 70.0),
            loadout_b: ffi_combat_loadout(Element::Earth, 240.0, 60.0),
            ..GochyaCombatMatchV1::default()
        }
    }

    #[test]
    fn abi_rejects_null() {
        assert_eq!(
            gochya_validate_heart_v1(std::ptr::null(), std::ptr::null_mut()),
            GochyaStatus::InvalidArgument
        );
    }

    #[test]
    fn abi_rejects_schema_mismatch() {
        let heart = GochyaHeartEvidenceV1 {
            schema_version: 99,
            ..ffi_heart()
        };
        let mut verdict = GochyaHeartVerdictV1::default();
        assert_eq!(
            gochya_validate_heart_v1(&raw const heart, &raw mut verdict),
            GochyaStatus::SchemaMismatch
        );
    }

    #[test]
    fn abi_writes_heart_verdict() {
        let heart = ffi_heart();
        let mut verdict = GochyaHeartVerdictV1::default();
        assert_eq!(
            gochya_validate_heart_v1(&raw const heart, &raw mut verdict),
            GochyaStatus::Ok
        );
        assert_eq!(verdict.passed, 1);
        assert_eq!(verdict.reason, HeartFailReason::Ok as u8);
        assert_eq!(verdict.struct_size, 28);
    }

    #[test]
    fn abi_derives_complete_technique_stats() {
        let metrics = GochyaPunchMetricsV1 {
            struct_size: size_u32::<GochyaPunchMetricsV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            technique_type: TechniqueType::Hook as u8,
            combo_len: 3,
            peak_accel_mps2: 65.0,
            exec_time_seconds: 0.5,
            precision: 0.8,
            rhythm_score: 0.75,
            ..GochyaPunchMetricsV1::default()
        };
        let heart = ffi_heart();
        let mut stats = GochyaTechniqueStatsV1::default();
        assert_eq!(
            gochya_derive_technique_v1(&raw const metrics, &raw const heart, 1.0, &raw mut stats),
            GochyaStatus::Ok
        );
        assert_eq!(stats.rarity, 2);
        assert!((stats.base_damage - 1.04).abs() < 0.000_01);
        assert_eq!(stats.quality, 64);
    }

    #[test]
    fn abi_combat_matches_domain_golden_result() {
        let match_ = ffi_combat_match();
        let mut result = GochyaCombatResultV1::default();
        assert_eq!(
            gochya_simulate_combat_v1(&raw const match_, 42, &raw mut result),
            GochyaStatus::Ok
        );
        assert_eq!(result.struct_size, 280);
        assert_eq!(result.schema_version, ABI_SCHEMA_VERSION);
        assert_eq!(result.winner, 0);
        assert_eq!(result.round_count, 3);
        assert_eq!(result.final_hp_a, 950);
        assert_eq!(result.final_hp_b, 0);
        assert_eq!(result.seed, 42);
        assert_eq!(
            result.rounds[..3]
                .iter()
                .map(|round| (round.damage_a_to_b, round.damage_b_to_a))
                .collect::<Vec<_>>(),
            vec![(437, 169), (453, 181), (413, 0)]
        );
        assert!(result.reserved.iter().all(|value| *value == 0));
    }

    #[test]
    fn abi_combat_rejects_invalid_envelope_and_domain_values() {
        let mut output = GochyaCombatResultV1::default();
        assert_eq!(
            gochya_simulate_combat_v1(std::ptr::null(), 42, &raw mut output),
            GochyaStatus::InvalidArgument
        );

        let mut match_ = ffi_combat_match();
        match_.schema_version = 99;
        assert_eq!(
            gochya_simulate_combat_v1(&raw const match_, 42, &raw mut output),
            GochyaStatus::SchemaMismatch
        );

        match_ = ffi_combat_match();
        match_.loadout_a.signature_idx = 5;
        assert_eq!(
            gochya_simulate_combat_v1(&raw const match_, 42, &raw mut output),
            GochyaStatus::InvalidArgument
        );

        match_ = ffi_combat_match();
        match_.loadout_a.cards[0].base_damage = f32::NAN;
        assert_eq!(
            gochya_simulate_combat_v1(&raw const match_, 42, &raw mut output),
            GochyaStatus::InvalidArgument
        );

        match_ = ffi_combat_match();
        match_.loadout_b.element = u8::MAX;
        assert_eq!(
            gochya_simulate_combat_v1(&raw const match_, 42, &raw mut output),
            GochyaStatus::InvalidArgument
        );
    }
}
