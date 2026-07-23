//! Stable C ABI. Domain structs deliberately do not cross this boundary.
#![allow(clippy::not_unsafe_ptr_arg_deref)]

use std::{mem::size_of, panic};

use crate::{
    HeartRateEvidence, PunchMetrics,
    heart::validate_heart,
    synergy::{DailyActivitySnapshot, DailyGoals, compute_vitality},
    technique::{TechniqueType, derive_technique_stats, quality_score},
};

pub const ABI_VERSION: u32 = 0x0001_0000;
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

const _: () = {
    assert!(size_of::<GochyaPunchMetricsV1>() == 40);
    assert!(size_of::<GochyaHeartEvidenceV1>() == 36);
    assert!(size_of::<GochyaHeartVerdictV1>() == 28);
    assert!(size_of::<GochyaDailyActivityV1>() == 32);
    assert!(size_of::<GochyaDailyGoalsV1>() == 32);
    assert!(size_of::<GochyaTechniqueStatsV1>() == 40);
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
}
