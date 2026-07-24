//! Deterministic gameplay rules shared by every GOCHYA client and the server.
//!
//! The idiomatic Rust API lives in the domain modules. Foreign-language hosts
//! must use the versioned, panic-safe functions from [`ffi`].

pub mod combat;
pub mod combat_ai;
pub mod ffi;
pub mod genome;
pub mod heart;
pub mod pet;
pub mod rng;
pub mod serde_helpers;
pub mod synergy;
pub mod technique;

pub use combat::{
    ActiveEffects, CombatantState, GearSummary, Loadout, Match, MatchMode, MatchResult, RoundLog,
    Winner, simulate_combat,
};
pub use genome::{
    Ability, BreedResult, Catalysts, Element, Genome, StatPotentials, VisualGenes, breed,
    generate_starter_genome, hybrid_of, mutation_chance, stat_cap_penalty,
};
pub use heart::{
    HeartFailReason, HeartRateEvidence, HeartVerdict, heart_score, spirit_bonus, validate_heart,
};
pub use pet::{Needs, Pet, Stage, Stats, mood_multiplier};
pub use rng::{Rng, rng_new, rng_next, rng_range};
pub use serde_helpers::{SCHEMA_VERSION, SchemaEnvelope};
pub use synergy::{
    DailyActivitySnapshot, DailyGoals, DataSource, MAX_VITALITY_PER_DAY, MAX_WORKOUTS,
    PersonalBaseline, StatGains, WorkoutKind, WorkoutSummary, compute_goals, compute_stat_gains,
    compute_vitality, resonance_bonus, synergy_multiplier,
};
pub use technique::{
    Effect, EffectKind, PunchMetrics, Rarity, TechniqueCard, TechniqueStats, TechniqueType,
    combo_score, crit_chance, derive_technique_stats, generate_loot_technique_stats,
    muscle_memory_bonus, norm_power, quality_score, rarity_from_quality, tech_card_bonus,
    type_multiplier,
};
