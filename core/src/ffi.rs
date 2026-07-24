//! Stable C ABI. Domain structs deliberately do not cross this boundary.
#![allow(clippy::not_unsafe_ptr_arg_deref)]

use std::{mem::size_of, panic};

use crate::{
    HeartRateEvidence, PunchMetrics,
    combat::{GearSummary, Loadout, Match, MatchMode, simulate_combat},
    genome::{
        Ability, Catalysts, Element, Genome, StatPotentials, VisualGenes, breed,
        generate_starter_genome,
    },
    heart::validate_heart,
    pet::{
        CareAction, CareItem, Needs, NeedsDecayRemainders, NeedsState, Stats, advance_needs,
        apply_care_action,
    },
    synergy::{
        DailyActivitySnapshot, DailyGoals, DataSource, MAX_WORKOUTS, PersonalBaseline,
        WorkoutSummary, compute_goals, compute_stat_gains, compute_vitality,
    },
    technique::{
        Effect, EffectKind, Rarity, TechniqueCard, TechniqueStats, TechniqueType,
        derive_technique_stats, generate_loot_technique_stats, quality_score,
    },
};

pub const ABI_VERSION: u32 = 0x0002_0300;
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
pub struct GochyaNeedsStateV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub is_sleeping: u8,
    pub is_weak: u8,
    pub hunger: u8,
    pub energy: u8,
    pub hygiene: u8,
    pub mood: u8,
    pub hunger_remainder: u32,
    pub energy_remainder: u32,
    pub hygiene_remainder: u32,
    pub mood_remainder: u32,
    pub zero_streak_seconds: u64,
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
pub struct GochyaPersonalBaselineV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub reserved0: [u8; 2],
    pub steps_14d_average: u32,
    pub sleep_hours_14d_average: f32,
    pub active_calories_14d_average: u16,
    pub reserved: [u8; 14],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaWorkoutV1 {
    pub kind: u8,
    pub reserved0: u8,
    pub duration_minutes: u16,
    pub calories: u16,
    pub reserved: [u8; 2],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaActivityInputV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub reserved0: [u8; 2],
    pub steps: u32,
    pub sleep_minutes: u16,
    pub active_calories: u16,
    pub sleep_quality: u8,
    pub workout_count: u8,
    pub stress_level: u8,
    pub stand_hours: u8,
    pub source: u8,
    pub pet_element: u8,
    pub reserved1: [u8; 2],
    pub avg_hr: u16,
    pub hr_zone_high_minutes: u16,
    pub meditation_minutes: u16,
    pub floors: u16,
    pub timestamp: u64,
    pub workouts: [GochyaWorkoutV1; 8],
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaActivityResultV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub vitality: u16,
    pub stat_str: i16,
    pub stat_agi: i16,
    pub stat_end: i16,
    pub stat_foc: i16,
    pub reserved: [u8; 16],
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

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaVisualGenesV1 {
    pub body_shape: u8,
    pub reserved0: u8,
    pub palette_hue: u16,
    pub palette_sat: u8,
    pub pattern: u8,
    pub size: u8,
    pub eye_style: u8,
    pub aura: u8,
    pub reserved: [u8; 7],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaStatPotentialsV1 {
    pub str_pot: u8,
    pub agi_pot: u8,
    pub end_pot: u8,
    pub foc_pot: u8,
    pub reserved: [u8; 4],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaGenomeV1 {
    pub visual: GochyaVisualGenesV1,
    pub stats: GochyaStatPotentialsV1,
    pub element: u8,
    pub tech_affinity: u8,
    pub rarity: u8,
    pub ability: u8,
    pub generation: u32,
    pub reserved: [u8; 8],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaBreedInputV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub mutation_catalyst: u8,
    pub hybrid_catalyst: u8,
    pub inbreeding_coeff: u8,
    pub reserved0: [u8; 7],
    pub parent_a: GochyaGenomeV1,
    pub parent_b: GochyaGenomeV1,
    pub reserved: [u8; 16],
}

#[derive(Clone, Copy, Debug, Default)]
#[repr(C)]
pub struct GochyaBreedResultV1 {
    pub struct_size: u32,
    pub schema_version: u16,
    pub incubation_hours: u8,
    pub reserved0: u8,
    pub genome: GochyaGenomeV1,
    pub mutated_genes: u16,
    pub reserved: [u8; 14],
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
pub extern "C" fn gochya_compute_goals_v1(
    baseline: *const GochyaPersonalBaselineV1,
    out_goals: *mut GochyaDailyGoalsV1,
) -> GochyaStatus {
    catch_status(|| {
        if baseline.is_null() || out_goals.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable/writable memory.
        let baseline = unsafe { &*baseline };
        if !valid_header(
            baseline.struct_size,
            baseline.schema_version,
            size_of::<GochyaPersonalBaselineV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        if !baseline.sleep_hours_14d_average.is_finite() {
            return GochyaStatus::InvalidArgument;
        }
        let goals = compute_goals(&PersonalBaseline {
            steps_14d_ma: baseline.steps_14d_average,
            sleep_14d_ma: baseline.sleep_hours_14d_average,
            cals_14d_ma: baseline.active_calories_14d_average,
        });
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe {
            *out_goals = GochyaDailyGoalsV1 {
                struct_size: size_u32::<GochyaDailyGoalsV1>(),
                schema_version: ABI_SCHEMA_VERSION,
                steps: goals.steps,
                sleep_hours: goals.sleep_hours,
                active_calories: goals.cals,
                ..GochyaDailyGoalsV1::default()
            };
        }
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_compute_activity_v1(
    activity: *const GochyaActivityInputV1,
    goals: *const GochyaDailyGoalsV1,
    streak_days: u32,
    out_result: *mut GochyaActivityResultV1,
) -> GochyaStatus {
    catch_status(|| {
        if activity.is_null() || goals.is_null() || out_result.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable V1 inputs.
        let (activity, goals) = unsafe { (&*activity, &*goals) };
        if !valid_header(
            activity.struct_size,
            activity.schema_version,
            size_of::<GochyaActivityInputV1>(),
        ) || !valid_header(
            goals.struct_size,
            goals.schema_version,
            size_of::<GochyaDailyGoalsV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        if !goals.sleep_hours.is_finite()
            || usize::from(activity.workout_count) > MAX_WORKOUTS
            || activity.sleep_quality > 100
            || activity.stress_level > 100
        {
            return GochyaStatus::InvalidArgument;
        }
        let Some(source) = data_source_from_u8(activity.source) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(element) = element_from_u8(activity.pet_element) else {
            return GochyaStatus::InvalidArgument;
        };
        let mut workouts = [WorkoutSummary::default(); MAX_WORKOUTS];
        for (output, input) in workouts.iter_mut().zip(activity.workouts) {
            *output = WorkoutSummary {
                kind: input.kind,
                duration_min: input.duration_minutes,
                calories: input.calories,
            };
        }
        let snapshot = DailyActivitySnapshot {
            steps: activity.steps,
            sleep_minutes: activity.sleep_minutes,
            sleep_quality: activity.sleep_quality,
            active_calories: activity.active_calories,
            workouts,
            workout_count: activity.workout_count,
            avg_hr: activity.avg_hr,
            hr_zone_high_min: activity.hr_zone_high_minutes,
            meditation_min: activity.meditation_minutes,
            stress_level: activity.stress_level,
            floors: activity.floors,
            stand_hours: activity.stand_hours,
            source,
            timestamp: activity.timestamp,
        };
        let domain_goals = DailyGoals {
            steps: goals.steps,
            sleep_hours: goals.sleep_hours,
            cals: goals.active_calories,
        };
        let genome = Genome {
            element,
            ..Genome::default()
        };
        let gains = compute_stat_gains(&snapshot, &domain_goals, &genome, streak_days);
        // SAFETY: output pointer was checked and caller promises it is writable.
        unsafe {
            *out_result = GochyaActivityResultV1 {
                struct_size: size_u32::<GochyaActivityResultV1>(),
                schema_version: ABI_SCHEMA_VERSION,
                vitality: compute_vitality(&snapshot, &domain_goals, streak_days),
                stat_str: gains.str,
                stat_agi: gains.agi,
                stat_end: gains.end,
                stat_foc: gains.foc,
                reserved: [0; 16],
            };
        }
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
        write_technique_stats(out_stats, stats);
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_generate_loot_technique_v1(
    seed: u64,
    max_rarity: u8,
    out_stats: *mut GochyaTechniqueStatsV1,
) -> GochyaStatus {
    catch_status(|| {
        if out_stats.is_null() || max_rarity > Rarity::Epic as u8 {
            return GochyaStatus::InvalidArgument;
        }
        let rarity = match max_rarity {
            0 => Rarity::Common,
            1 => Rarity::Uncommon,
            2 => Rarity::Rare,
            _ => Rarity::Epic,
        };
        write_technique_stats(out_stats, generate_loot_technique_stats(seed, rarity));
        GochyaStatus::Ok
    })
}

fn write_technique_stats(out_stats: *mut GochyaTechniqueStatsV1, stats: TechniqueStats) {
    // SAFETY: every caller checks that the output pointer is non-null and the
    // foreign caller promises writable storage for the complete V1 struct.
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

#[unsafe(no_mangle)]
pub extern "C" fn gochya_breed_v1(
    input: *const GochyaBreedInputV1,
    seed: u64,
    out_result: *mut GochyaBreedResultV1,
) -> GochyaStatus {
    catch_status(|| {
        if input.is_null() || out_result.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and caller promises readable/writable memory.
        let input = unsafe { &*input };
        if !valid_header(
            input.struct_size,
            input.schema_version,
            size_of::<GochyaBreedInputV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        if input.mutation_catalyst > 1 || input.hybrid_catalyst > 1 || input.inbreeding_coeff > 3 {
            return GochyaStatus::InvalidArgument;
        }
        let Some(parent_a) = genome_from_ffi(&input.parent_a) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(parent_b) = genome_from_ffi(&input.parent_b) else {
            return GochyaStatus::InvalidArgument;
        };
        let result = breed(
            &parent_a,
            &parent_b,
            &Catalysts {
                mutation: input.mutation_catalyst != 0,
                hybrid: input.hybrid_catalyst != 0,
            },
            input.inbreeding_coeff,
            seed,
        );
        // SAFETY: output pointer was checked and caller promises writable storage.
        unsafe {
            *out_result = GochyaBreedResultV1 {
                struct_size: size_u32::<GochyaBreedResultV1>(),
                schema_version: ABI_SCHEMA_VERSION,
                incubation_hours: result.incubation_hours,
                reserved0: 0,
                genome: genome_to_ffi(result.genome),
                mutated_genes: result.mutated_genes,
                reserved: [0; 14],
            };
        }
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_generate_starter_genome_v1(
    element: u8,
    seed: u64,
    out_genome: *mut GochyaGenomeV1,
) -> GochyaStatus {
    catch_status(|| {
        if out_genome.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        let starter_element = match element {
            0 => Element::Fire,
            1 => Element::Water,
            2 => Element::Earth,
            _ => return GochyaStatus::InvalidArgument,
        };
        let Some(genome) = generate_starter_genome(starter_element, seed) else {
            return GochyaStatus::DomainRejected;
        };
        // SAFETY: the output pointer was checked and the caller promises
        // writable storage for the complete V1 struct.
        unsafe {
            *out_genome = genome_to_ffi(genome);
        }
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_advance_needs_v1(
    input: *const GochyaNeedsStateV1,
    elapsed_seconds: u64,
    out_state: *mut GochyaNeedsStateV1,
) -> GochyaStatus {
    catch_status(|| {
        if input.is_null() || out_state.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and the caller promises readable memory.
        let input = unsafe { &*input };
        if !valid_header(
            input.struct_size,
            input.schema_version,
            size_of::<GochyaNeedsStateV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        let Some(state) = needs_state_from_ffi(input) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(result) = advance_needs(state, elapsed_seconds) else {
            return GochyaStatus::DomainRejected;
        };
        write_needs_state(out_state, result);
        GochyaStatus::Ok
    })
}

#[unsafe(no_mangle)]
pub extern "C" fn gochya_apply_care_v1(
    input: *const GochyaNeedsStateV1,
    action: u8,
    item: u8,
    out_state: *mut GochyaNeedsStateV1,
) -> GochyaStatus {
    catch_status(|| {
        if input.is_null() || out_state.is_null() {
            return GochyaStatus::InvalidArgument;
        }
        // SAFETY: pointers are non-null and the caller promises readable memory.
        let input = unsafe { &*input };
        if !valid_header(
            input.struct_size,
            input.schema_version,
            size_of::<GochyaNeedsStateV1>(),
        ) {
            return GochyaStatus::SchemaMismatch;
        }
        let Some(state) = needs_state_from_ffi(input) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(action) = care_action_from_u8(action) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(item) = care_item_from_u8(item) else {
            return GochyaStatus::InvalidArgument;
        };
        let Some(result) = apply_care_action(state, action, item) else {
            return GochyaStatus::DomainRejected;
        };
        write_needs_state(out_state, result);
        GochyaStatus::Ok
    })
}

fn needs_state_from_ffi(input: &GochyaNeedsStateV1) -> Option<NeedsState> {
    if input.is_sleeping > 1
        || input.is_weak > 1
        || input.hunger > 100
        || input.energy > 100
        || input.hygiene > 100
        || input.mood > 100
    {
        return None;
    }
    Some(NeedsState {
        needs: Needs {
            hunger: input.hunger,
            energy: input.energy,
            hygiene: input.hygiene,
            mood: input.mood,
        },
        remainders: NeedsDecayRemainders {
            hunger: input.hunger_remainder,
            energy: input.energy_remainder,
            hygiene: input.hygiene_remainder,
            mood: input.mood_remainder,
        },
        zero_streak_seconds: input.zero_streak_seconds,
        is_sleeping: input.is_sleeping != 0,
        is_weak: input.is_weak != 0,
    })
}

fn write_needs_state(out_state: *mut GochyaNeedsStateV1, state: NeedsState) {
    // SAFETY: callers check the output pointer and promise complete writable storage.
    unsafe {
        *out_state = GochyaNeedsStateV1 {
            struct_size: size_u32::<GochyaNeedsStateV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            is_sleeping: u8::from(state.is_sleeping),
            is_weak: u8::from(state.is_weak),
            hunger: state.needs.hunger,
            energy: state.needs.energy,
            hygiene: state.needs.hygiene,
            mood: state.needs.mood,
            hunger_remainder: state.remainders.hunger,
            energy_remainder: state.remainders.energy,
            hygiene_remainder: state.remainders.hygiene,
            mood_remainder: state.remainders.mood,
            zero_streak_seconds: state.zero_streak_seconds,
            reserved: [0; 16],
        };
    }
}

fn genome_from_ffi(input: &GochyaGenomeV1) -> Option<Genome> {
    if input.visual.palette_hue > 360
        || input.visual.palette_sat > 100
        || input.stats.str_pot > 100
        || input.stats.agi_pot > 100
        || input.stats.end_pot > 100
        || input.stats.foc_pot > 100
    {
        return None;
    }
    Some(Genome {
        visual: VisualGenes {
            body_shape: input.visual.body_shape,
            palette_hue: input.visual.palette_hue,
            palette_sat: input.visual.palette_sat,
            pattern: input.visual.pattern,
            size: input.visual.size,
            eye_style: input.visual.eye_style,
            aura: input.visual.aura,
        },
        stats: StatPotentials {
            str_pot: input.stats.str_pot,
            agi_pot: input.stats.agi_pot,
            end_pot: input.stats.end_pot,
            foc_pot: input.stats.foc_pot,
        },
        element: element_from_u8(input.element)?,
        tech_affinity: technique_type_from_u8(input.tech_affinity)?,
        rarity: rarity_from_u8(input.rarity)?,
        ability: ability_from_u8(input.ability)?,
        generation: input.generation,
    })
}

fn genome_to_ffi(input: Genome) -> GochyaGenomeV1 {
    GochyaGenomeV1 {
        visual: GochyaVisualGenesV1 {
            body_shape: input.visual.body_shape,
            palette_hue: input.visual.palette_hue,
            palette_sat: input.visual.palette_sat,
            pattern: input.visual.pattern,
            size: input.visual.size,
            eye_style: input.visual.eye_style,
            aura: input.visual.aura,
            ..GochyaVisualGenesV1::default()
        },
        stats: GochyaStatPotentialsV1 {
            str_pot: input.stats.str_pot,
            agi_pot: input.stats.agi_pot,
            end_pot: input.stats.end_pot,
            foc_pot: input.stats.foc_pot,
            ..GochyaStatPotentialsV1::default()
        },
        element: input.element as u8,
        tech_affinity: input.tech_affinity as u8,
        rarity: input.rarity as u8,
        ability: input.ability as u8,
        generation: input.generation,
        reserved: [0; 8],
    }
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

const fn data_source_from_u8(value: u8) -> Option<DataSource> {
    match value {
        0 => Some(DataSource::Watch),
        1 => Some(DataSource::Phone),
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

const fn rarity_from_u8(value: u8) -> Option<Rarity> {
    match value {
        0 => Some(Rarity::Common),
        1 => Some(Rarity::Uncommon),
        2 => Some(Rarity::Rare),
        3 => Some(Rarity::Epic),
        4 => Some(Rarity::Legendary),
        5 => Some(Rarity::Mythic),
        _ => None,
    }
}

const fn ability_from_u8(value: u8) -> Option<Ability> {
    match value {
        0 => Some(Ability::None),
        1 => Some(Ability::Regen),
        2 => Some(Ability::CritAura),
        3 => Some(Ability::Thorns),
        4 => Some(Ability::Shield),
        5 => Some(Ability::Lifesteal),
        6 => Some(Ability::LineageSignature),
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

const fn care_action_from_u8(value: u8) -> Option<CareAction> {
    match value {
        0 => Some(CareAction::Feed),
        1 => Some(CareAction::Clean),
        2 => Some(CareAction::Play),
        3 => Some(CareAction::Sleep),
        _ => None,
    }
}

const fn care_item_from_u8(value: u8) -> Option<CareItem> {
    match value {
        0 => Some(CareItem::None),
        1 => Some(CareItem::Apple),
        2 => Some(CareItem::Steak),
        3 => Some(CareItem::EnergyDrink),
        4 => Some(CareItem::Soap),
        5 => Some(CareItem::Shampoo),
        _ => None,
    }
}

const _: () = {
    assert!(MAX_WORKOUTS == 8);
    assert!(size_of::<GochyaPunchMetricsV1>() == 40);
    assert!(size_of::<GochyaHeartEvidenceV1>() == 36);
    assert!(size_of::<GochyaHeartVerdictV1>() == 28);
    assert!(size_of::<GochyaNeedsStateV1>() == 56);
    assert!(size_of::<GochyaDailyActivityV1>() == 32);
    assert!(size_of::<GochyaDailyGoalsV1>() == 32);
    assert!(size_of::<GochyaPersonalBaselineV1>() == 32);
    assert!(size_of::<GochyaWorkoutV1>() == 8);
    assert!(size_of::<GochyaActivityInputV1>() == 120);
    assert!(size_of::<GochyaActivityResultV1>() == 32);
    assert!(size_of::<GochyaTechniqueStatsV1>() == 40);
    assert!(size_of::<GochyaCombatCardV1>() == 20);
    assert!(size_of::<GochyaCombatLoadoutV1>() == 144);
    assert!(size_of::<GochyaCombatMatchV1>() == 312);
    assert!(size_of::<GochyaCombatRoundV1>() == 12);
    assert!(size_of::<GochyaCombatResultV1>() == 280);
    assert!(size_of::<GochyaVisualGenesV1>() == 16);
    assert!(size_of::<GochyaStatPotentialsV1>() == 8);
    assert!(size_of::<GochyaGenomeV1>() == 40);
    assert!(size_of::<GochyaBreedInputV1>() == 112);
    assert!(size_of::<GochyaBreedResultV1>() == 64);
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

    fn ffi_activity() -> GochyaActivityInputV1 {
        let mut workouts = [GochyaWorkoutV1::default(); MAX_WORKOUTS];
        workouts[0] = GochyaWorkoutV1 {
            kind: crate::WorkoutKind::Strength as u8,
            duration_minutes: 30,
            calories: 150,
            ..GochyaWorkoutV1::default()
        };
        workouts[1] = GochyaWorkoutV1 {
            kind: crate::WorkoutKind::Running as u8,
            duration_minutes: 30,
            calories: 200,
            ..GochyaWorkoutV1::default()
        };
        workouts[2] = GochyaWorkoutV1 {
            kind: crate::WorkoutKind::Yoga as u8,
            duration_minutes: 60,
            calories: 150,
            ..GochyaWorkoutV1::default()
        };
        GochyaActivityInputV1 {
            struct_size: size_u32::<GochyaActivityInputV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            steps: 10_000,
            sleep_minutes: 480,
            active_calories: 500,
            sleep_quality: 100,
            workout_count: 3,
            stress_level: 20,
            source: DataSource::Watch as u8,
            pet_element: Element::Earth as u8,
            hr_zone_high_minutes: 10,
            meditation_minutes: 15,
            floors: 10,
            workouts,
            ..GochyaActivityInputV1::default()
        }
    }

    fn ffi_goals() -> GochyaDailyGoalsV1 {
        GochyaDailyGoalsV1 {
            struct_size: size_u32::<GochyaDailyGoalsV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            steps: 10_000,
            sleep_hours: 8.0,
            active_calories: 500,
            ..GochyaDailyGoalsV1::default()
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

    fn ffi_genome(element: Element, generation: u32, offset: u8) -> GochyaGenomeV1 {
        GochyaGenomeV1 {
            visual: GochyaVisualGenesV1 {
                body_shape: offset,
                palette_hue: 30 + u16::from(offset),
                palette_sat: 60 + offset,
                pattern: offset,
                size: offset,
                eye_style: offset,
                aura: offset,
                ..GochyaVisualGenesV1::default()
            },
            stats: GochyaStatPotentialsV1 {
                str_pot: 50 + offset,
                agi_pot: 60 + offset,
                end_pot: 70 + offset,
                foc_pot: 80 + offset,
                ..GochyaStatPotentialsV1::default()
            },
            element: element as u8,
            tech_affinity: TechniqueType::Hook as u8,
            rarity: Rarity::Rare as u8,
            ability: Ability::Regen as u8,
            generation,
            reserved: [0; 8],
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
        assert!((stats.base_damage - 104.0).abs() < 0.000_01);
        assert_eq!(stats.quality, 64);
    }

    #[test]
    fn abi_generates_deterministic_capped_loot_technique() {
        let mut first = GochyaTechniqueStatsV1::default();
        let mut second = GochyaTechniqueStatsV1::default();
        assert_eq!(
            gochya_generate_loot_technique_v1(42, Rarity::Rare as u8, &raw mut first),
            GochyaStatus::Ok
        );
        assert_eq!(
            gochya_generate_loot_technique_v1(42, Rarity::Rare as u8, &raw mut second),
            GochyaStatus::Ok
        );
        assert_eq!(first.technique_type, second.technique_type);
        assert_eq!(first.rarity, second.rarity);
        assert_eq!(first.base_damage, second.base_damage);
        assert_eq!(first.quality, second.quality);
        assert_eq!(first.technique_type, TechniqueType::Elbow as u8);
        assert_eq!(first.rarity, Rarity::Common as u8);
        assert_eq!(first.base_damage, 126.5);
        assert_eq!(first.quality, 35);
        assert!(first.rarity <= Rarity::Rare as u8);
        assert_eq!(
            gochya_generate_loot_technique_v1(42, Rarity::Legendary as u8, &raw mut first),
            GochyaStatus::InvalidArgument
        );
    }

    #[test]
    fn abi_computes_complete_activity_result() {
        let activity = ffi_activity();
        let goals = ffi_goals();
        let mut result = GochyaActivityResultV1::default();
        assert_eq!(
            gochya_compute_activity_v1(&raw const activity, &raw const goals, 10, &raw mut result,),
            GochyaStatus::Ok
        );
        assert_eq!(result.struct_size, 32);
        assert_eq!(result.schema_version, ABI_SCHEMA_VERSION);
        assert_eq!(result.vitality, 104);
        assert_eq!(
            (
                result.stat_str,
                result.stat_agi,
                result.stat_end,
                result.stat_foc
            ),
            (7, 7, 12, 7)
        );
        assert!(result.reserved.iter().all(|value| *value == 0));
    }

    #[test]
    fn abi_computes_adaptive_activity_goals() {
        let baseline = GochyaPersonalBaselineV1 {
            struct_size: size_u32::<GochyaPersonalBaselineV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            steps_14d_average: 8_000,
            sleep_hours_14d_average: 7.0,
            active_calories_14d_average: 400,
            ..GochyaPersonalBaselineV1::default()
        };
        let mut goals = GochyaDailyGoalsV1::default();
        assert_eq!(
            gochya_compute_goals_v1(&raw const baseline, &raw mut goals),
            GochyaStatus::Ok
        );
        assert_eq!(goals.struct_size, 32);
        assert_eq!(goals.schema_version, ABI_SCHEMA_VERSION);
        assert_eq!(goals.steps, 9_200);
        assert!((goals.sleep_hours - 7.7).abs() < 0.000_01);
        assert_eq!(goals.active_calories, 460);
        assert!(goals.reserved.iter().all(|value| *value == 0));
    }

    #[test]
    fn abi_activity_rejects_invalid_envelope_and_domain_values() {
        let goals = ffi_goals();
        let mut output = GochyaActivityResultV1::default();
        assert_eq!(
            gochya_compute_activity_v1(std::ptr::null(), &raw const goals, 10, &raw mut output),
            GochyaStatus::InvalidArgument
        );

        let mut activity = ffi_activity();
        activity.schema_version = 99;
        assert_eq!(
            gochya_compute_activity_v1(&raw const activity, &raw const goals, 10, &raw mut output,),
            GochyaStatus::SchemaMismatch
        );

        activity = ffi_activity();
        activity.workout_count = (MAX_WORKOUTS + 1) as u8;
        assert_eq!(
            gochya_compute_activity_v1(&raw const activity, &raw const goals, 10, &raw mut output,),
            GochyaStatus::InvalidArgument
        );

        activity = ffi_activity();
        activity.source = 2;
        assert_eq!(
            gochya_compute_activity_v1(&raw const activity, &raw const goals, 10, &raw mut output,),
            GochyaStatus::InvalidArgument
        );

        activity = ffi_activity();
        activity.pet_element = u8::MAX;
        assert_eq!(
            gochya_compute_activity_v1(&raw const activity, &raw const goals, 10, &raw mut output,),
            GochyaStatus::InvalidArgument
        );
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

    #[test]
    fn abi_breeding_is_deterministic_and_validates_inputs() {
        let input = GochyaBreedInputV1 {
            struct_size: size_u32::<GochyaBreedInputV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            mutation_catalyst: 1,
            hybrid_catalyst: 1,
            parent_a: ffi_genome(Element::Fire, 2, 1),
            parent_b: ffi_genome(Element::Water, 5, 2),
            ..GochyaBreedInputV1::default()
        };
        let mut first = GochyaBreedResultV1::default();
        let mut second = GochyaBreedResultV1::default();
        assert_eq!(
            gochya_breed_v1(&raw const input, 42, &raw mut first),
            GochyaStatus::Ok
        );
        assert_eq!(
            gochya_breed_v1(&raw const input, 42, &raw mut second),
            GochyaStatus::Ok
        );
        assert_eq!(first.struct_size, 64);
        assert_eq!(first.schema_version, ABI_SCHEMA_VERSION);
        assert_eq!(first.incubation_hours, second.incubation_hours);
        assert_eq!(first.mutated_genes, second.mutated_genes);
        assert_eq!(first.genome.generation, 6);
        assert_eq!(first.genome.element, second.genome.element);
        assert!((4..=24).contains(&first.incubation_hours));
        assert_eq!(first.mutated_genes & !0x3fff, 0);
        assert!(first.reserved.iter().all(|value| *value == 0));

        let mut invalid = input;
        invalid.parent_a.visual.palette_hue = 361;
        assert_eq!(
            gochya_breed_v1(&raw const invalid, 42, &raw mut first),
            GochyaStatus::InvalidArgument
        );
        invalid = input;
        invalid.schema_version = 99;
        assert_eq!(
            gochya_breed_v1(&raw const invalid, 42, &raw mut first),
            GochyaStatus::SchemaMismatch
        );
        assert_eq!(
            gochya_breed_v1(std::ptr::null(), 42, &raw mut first),
            GochyaStatus::InvalidArgument
        );
    }

    #[test]
    fn abi_starter_genome_is_deterministic_and_rejects_non_starter_elements() {
        let mut first = GochyaGenomeV1::default();
        let mut second = GochyaGenomeV1::default();
        assert_eq!(
            gochya_generate_starter_genome_v1(1, 42, &raw mut first),
            GochyaStatus::Ok
        );
        assert_eq!(
            gochya_generate_starter_genome_v1(1, 42, &raw mut second),
            GochyaStatus::Ok
        );
        assert_eq!(first.element, Element::Water as u8);
        assert_eq!(first.visual.body_shape, 1);
        assert_eq!(first.visual.palette_hue, 195);
        assert_eq!(first.rarity, Rarity::Common as u8);
        assert_eq!(first.ability, Ability::None as u8);
        assert_eq!(first.generation, 0);
        assert_eq!(first.visual.body_shape, second.visual.body_shape);
        assert_eq!(first.visual.pattern, second.visual.pattern);
        assert_eq!(first.visual.size, second.visual.size);
        assert_eq!(first.visual.eye_style, second.visual.eye_style);
        assert_eq!(first.tech_affinity, second.tech_affinity);
        assert_eq!(
            gochya_generate_starter_genome_v1(3, 42, &raw mut first),
            GochyaStatus::InvalidArgument
        );
        assert_eq!(
            gochya_generate_starter_genome_v1(0, 42, std::ptr::null_mut()),
            GochyaStatus::InvalidArgument
        );
    }

    #[test]
    fn abi_needs_decay_and_care_are_bounded_and_deterministic() {
        let input = GochyaNeedsStateV1 {
            struct_size: size_u32::<GochyaNeedsStateV1>(),
            schema_version: ABI_SCHEMA_VERSION,
            hunger: 100,
            energy: 100,
            hygiene: 100,
            mood: 100,
            ..GochyaNeedsStateV1::default()
        };
        let mut first = GochyaNeedsStateV1::default();
        let mut second = GochyaNeedsStateV1::default();
        assert_eq!(
            gochya_advance_needs_v1(&raw const input, 86_400, &raw mut first),
            GochyaStatus::Ok
        );
        assert_eq!(
            gochya_advance_needs_v1(&raw const input, 86_400, &raw mut second),
            GochyaStatus::Ok
        );
        assert_eq!(first.hunger, 76);
        assert_eq!(first.energy, 84);
        assert_eq!(first.hygiene, 88);
        assert_eq!(first.mood, 100);
        assert_eq!(first.hunger, second.hunger);
        assert_eq!(first.energy_remainder, second.energy_remainder);
        assert_eq!(
            gochya_advance_needs_v1(&raw const input, 86_401, &raw mut first),
            GochyaStatus::DomainRejected
        );

        let care_input = GochyaNeedsStateV1 {
            hunger: 50,
            energy: 10,
            hygiene: 10,
            mood: 90,
            ..input
        };
        assert_eq!(
            gochya_apply_care_v1(&raw const care_input, 0, 2, &raw mut first),
            GochyaStatus::Ok
        );
        assert_eq!(first.hunger, 100);
        assert_eq!(first.mood, 95);
        assert_eq!(
            gochya_apply_care_v1(&raw const care_input, 0, 4, &raw mut first),
            GochyaStatus::DomainRejected
        );
        assert_eq!(
            gochya_apply_care_v1(&raw const care_input, 9, 0, &raw mut first),
            GochyaStatus::InvalidArgument
        );
    }
}
