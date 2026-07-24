use serde::{Deserialize, Serialize};

use crate::genome::Genome;

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum Stage {
    #[default]
    Egg = 0,
    Baby = 1,
    Teen = 2,
    Adult = 3,
    Premium = 4,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct Needs {
    pub hunger: u8,
    pub energy: u8,
    pub hygiene: u8,
    pub mood: u8,
}

pub const MAX_NEEDS_ADVANCE_SECONDS: u64 = 24 * 60 * 60;
pub const WEAKNESS_AFTER_SECONDS: u64 = 6 * 60 * 60;
const DECAY_DENOMINATOR: u32 = 10_800_000;

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct NeedsDecayRemainders {
    pub hunger: u32,
    pub energy: u32,
    pub hygiene: u32,
    pub mood: u32,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct NeedsState {
    pub needs: Needs,
    pub remainders: NeedsDecayRemainders,
    pub zero_streak_seconds: u64,
    pub is_sleeping: bool,
    pub is_weak: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(u8)]
pub enum CareAction {
    Feed = 0,
    Clean = 1,
    Play = 2,
    Sleep = 3,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
#[repr(u8)]
pub enum CareItem {
    #[default]
    None = 0,
    Apple = 1,
    Steak = 2,
    EnergyDrink = 3,
    Soap = 4,
    Shampoo = 5,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct Stats {
    pub str: u32,
    pub agi: u32,
    pub end: u32,
    pub foc: u32,
}

impl Stats {
    #[must_use]
    pub const fn sum(self) -> u64 {
        self.str as u64 + self.agi as u64 + self.end as u64 + self.foc as u64
    }
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct Pet {
    pub id: [u8; 16],
    pub genome: Genome,
    pub name: [u8; 32],
    pub stage: Stage,
    pub level: u32,
    pub xp: u64,
    pub needs: Needs,
    pub stats: Stats,
    pub created_at: u64,
    pub last_sync: u64,
    pub last_bred_at: u64,
    pub needs_zero_since: u64,
    pub is_weak: bool,
}

/// Multiplier applied to stat gains and combat damage for the current mood.
#[must_use]
pub fn mood_multiplier(mood: u8) -> f32 {
    0.7_f32 + 0.3_f32 * (f32::from(mood.min(100)) / 100.0_f32)
}

/// Advances authoritative pet needs by at most one day.
///
/// Fixed-point remainders make the result independent of sync chunking. Longer
/// offline windows are processed by consumers in one-day chunks.
#[must_use]
pub fn advance_needs(mut state: NeedsState, elapsed_seconds: u64) -> Option<NeedsState> {
    if elapsed_seconds > MAX_NEEDS_ADVANCE_SECONDS || !valid_needs_state(state) {
        return None;
    }
    let (hunger_rate, energy_rate, hygiene_rate, mood_rate) = if state.is_sleeping {
        (1_000, 670, 500, 500)
    } else {
        (3_000, 2_010, 1_500, 1_500)
    };
    for _ in 0..elapsed_seconds {
        decay_need(
            &mut state.needs.hunger,
            &mut state.remainders.hunger,
            hunger_rate,
        );
        decay_need(
            &mut state.needs.energy,
            &mut state.remainders.energy,
            energy_rate,
        );
        decay_need(
            &mut state.needs.hygiene,
            &mut state.remainders.hygiene,
            hygiene_rate,
        );
        if state.needs.hunger == 0 || state.needs.energy == 0 || state.needs.hygiene == 0 {
            decay_need(&mut state.needs.mood, &mut state.remainders.mood, mood_rate);
        }
        if any_need_zero(state.needs) {
            state.zero_streak_seconds = state.zero_streak_seconds.saturating_add(1);
            if state.zero_streak_seconds >= WEAKNESS_AFTER_SECONDS {
                state.is_weak = true;
            }
        } else {
            state.zero_streak_seconds = 0;
        }
    }
    Some(state)
}

/// Applies one release-scoped care command without consuming inventory.
///
/// Inventory ownership and atomic consumption stay server-side; Core owns the
/// effect table and rejects action/item combinations outside the release.
#[must_use]
pub fn apply_care_action(
    mut state: NeedsState,
    action: CareAction,
    item: CareItem,
) -> Option<NeedsState> {
    if !valid_needs_state(state) {
        return None;
    }
    match (action, item) {
        (CareAction::Feed, CareItem::Apple) => {
            add_need(&mut state.needs.hunger, 20);
        }
        (CareAction::Feed, CareItem::Steak) => {
            add_need(&mut state.needs.hunger, 60);
            add_need(&mut state.needs.mood, 5);
        }
        (CareAction::Feed, CareItem::EnergyDrink) => {
            add_need(&mut state.needs.energy, 40);
        }
        (CareAction::Clean, CareItem::None) => {
            add_need(&mut state.needs.hygiene, 20);
        }
        (CareAction::Clean, CareItem::Soap) => {
            add_need(&mut state.needs.hygiene, 50);
        }
        (CareAction::Clean, CareItem::Shampoo) => {
            add_need(&mut state.needs.hygiene, 80);
            add_need(&mut state.needs.mood, 5);
        }
        (CareAction::Play, CareItem::None) => {
            add_need(&mut state.needs.mood, 20);
            state.needs.energy = state.needs.energy.saturating_sub(5);
        }
        (CareAction::Sleep, CareItem::None) => {}
        _ => return None,
    }
    state.is_sleeping = action == CareAction::Sleep;
    if !any_need_zero(state.needs) {
        state.zero_streak_seconds = 0;
    }
    if all_needs_at_least(state.needs, 50) {
        state.is_weak = false;
    }
    Some(state)
}

fn decay_need(value: &mut u8, remainder: &mut u32, rate: u32) {
    if *value == 0 {
        *remainder = 0;
        return;
    }
    let accumulated = *remainder + rate;
    let decrease = accumulated / DECAY_DENOMINATOR;
    *remainder = accumulated % DECAY_DENOMINATOR;
    if decrease != 0 {
        *value = value.saturating_sub(u8::try_from(decrease).unwrap_or(u8::MAX));
        if *value == 0 {
            *remainder = 0;
        }
    }
}

fn add_need(value: &mut u8, amount: u8) {
    *value = value.saturating_add(amount).min(100);
}

const fn any_need_zero(needs: Needs) -> bool {
    needs.hunger == 0 || needs.energy == 0 || needs.hygiene == 0 || needs.mood == 0
}

const fn all_needs_at_least(needs: Needs, minimum: u8) -> bool {
    needs.hunger >= minimum
        && needs.energy >= minimum
        && needs.hygiene >= minimum
        && needs.mood >= minimum
}

const fn valid_needs_state(state: NeedsState) -> bool {
    state.needs.hunger <= 100
        && state.needs.energy <= 100
        && state.needs.hygiene <= 100
        && state.needs.mood <= 100
        && state.remainders.hunger < DECAY_DENOMINATOR
        && state.remainders.energy < DECAY_DENOMINATOR
        && state.remainders.hygiene < DECAY_DENOMINATOR
        && state.remainders.mood < DECAY_DENOMINATOR
}

#[cfg(test)]
mod tests {
    use super::{
        CareAction, CareItem, Needs, NeedsState, WEAKNESS_AFTER_SECONDS, advance_needs,
        apply_care_action, mood_multiplier,
    };

    #[test]
    fn mood_multiplier_is_bounded() {
        assert_eq!(mood_multiplier(0), 0.7);
        assert_eq!(mood_multiplier(100), 1.0);
        assert_eq!(mood_multiplier(u8::MAX), 1.0);
    }

    #[test]
    fn needs_decay_is_deterministic_and_chunk_independent() {
        let initial = NeedsState {
            needs: Needs {
                hunger: 100,
                energy: 100,
                hygiene: 100,
                mood: 100,
            },
            ..NeedsState::default()
        };
        let whole = advance_needs(initial, 24 * 60 * 60).expect("one-day decay");
        let mut chunked = initial;
        for _ in 0..24 {
            chunked = advance_needs(chunked, 60 * 60).expect("hour decay");
        }
        assert_eq!(whole, chunked);
        assert_eq!(
            whole.needs,
            Needs {
                hunger: 76,
                energy: 84,
                hygiene: 88,
                mood: 100,
            }
        );

        let sleeping = advance_needs(
            NeedsState {
                is_sleeping: true,
                ..initial
            },
            24 * 60 * 60,
        )
        .expect("sleeping decay");
        assert_eq!(
            sleeping.needs,
            Needs {
                hunger: 92,
                energy: 95,
                hygiene: 96,
                mood: 100,
            }
        );
    }

    #[test]
    fn weakness_tracks_continuous_zero_time_and_care_recovery() {
        let initial = NeedsState {
            needs: Needs {
                hunger: 1,
                energy: 100,
                hygiene: 100,
                mood: 100,
            },
            ..NeedsState::default()
        };
        let before =
            advance_needs(initial, 60 * 60 + WEAKNESS_AFTER_SECONDS - 2).expect("pre-weak decay");
        assert!(!before.is_weak);
        let weak = advance_needs(before, 1).expect("weakness boundary");
        assert!(weak.is_weak);
        assert_eq!(weak.zero_streak_seconds, WEAKNESS_AFTER_SECONDS);

        let fed =
            apply_care_action(weak, CareAction::Feed, CareItem::Steak).expect("valid care action");
        assert!(!fed.is_weak);
        assert_eq!(fed.zero_streak_seconds, 0);
        assert_eq!(fed.needs.hunger, 60);
    }

    #[test]
    fn care_effects_are_bounded_and_release_scoped() {
        let initial = NeedsState {
            needs: Needs {
                hunger: 90,
                energy: 10,
                hygiene: 10,
                mood: 90,
            },
            ..NeedsState::default()
        };
        let fed =
            apply_care_action(initial, CareAction::Feed, CareItem::Steak).expect("steak is food");
        assert_eq!(fed.needs.hunger, 100);
        assert_eq!(fed.needs.mood, 95);
        let cleaned = apply_care_action(initial, CareAction::Clean, CareItem::Shampoo)
            .expect("shampoo is cleaning");
        assert_eq!(cleaned.needs.hygiene, 90);
        assert_eq!(cleaned.needs.mood, 95);
        let played = apply_care_action(initial, CareAction::Play, CareItem::None).expect("play");
        assert_eq!(played.needs.energy, 5);
        assert_eq!(played.needs.mood, 100);
        assert_eq!(
            apply_care_action(initial, CareAction::Feed, CareItem::Soap),
            None
        );
    }
}
