use serde::{Deserialize, Serialize};

use crate::{
    combat_ai::select_card_ai,
    genome::{Element, Genome},
    pet::{Stats, mood_multiplier},
    rng::{Rng, rng_new, rng_unit_f32},
    technique::{Effect, EffectKind, TechniqueCard, finite_or_zero, tech_card_bonus},
};

pub const MAX_ROUNDS: usize = 20;
pub const SIGNATURE_COOLDOWN_ROUNDS: u8 = 5;
pub const BLEED_DAMAGE_PER_STACK: i32 = 8;
pub const SLOW_INITIATIVE_PENALTY: f32 = 20.0;

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct GearSummary {
    pub str_bonus: i16,
    pub agi_bonus: i16,
    pub end_bonus: i16,
    pub foc_bonus: i16,
    pub element: Element,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct Loadout {
    pub pet_id: [u8; 16],
    pub pet_stats: Stats,
    pub pet_genome: Genome,
    pub pet_mood: u8,
    pub cards: [TechniqueCard; 5],
    pub signature_idx: u8,
    pub gear: GearSummary,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum MatchMode {
    #[default]
    Casual = 0,
    Ranked = 1,
    Tournament = 2,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct Match {
    pub loadout_a: Loadout,
    pub loadout_b: Loadout,
    pub mode: MatchMode,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum Winner {
    #[default]
    A = 0,
    B = 1,
    Draw = 2,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct RoundLog {
    pub card_a_idx: u8,
    pub card_b_idx: u8,
    pub damage_a_to_b: u16,
    pub damage_b_to_a: u16,
    pub effect_triggered: Effect,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct MatchResult {
    pub winner: Winner,
    pub rounds: [RoundLog; MAX_ROUNDS],
    pub round_count: u8,
    pub final_hp_a: u16,
    pub final_hp_b: u16,
    pub seed: u64,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct ActiveEffects {
    pub stun_rounds: u8,
    pub bleed_stacks: u8,
    pub slow_rounds: u8,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct CombatantState {
    pub hp: i32,
    pub stamina: i32,
    pub active: ActiveEffects,
    pub signature_cd: u8,
}

#[derive(Clone, Copy, Debug, Default)]
struct TurnModifiers {
    stunned: bool,
    slowed: bool,
}

#[derive(Clone, Copy, Debug, Default)]
struct ActionResult {
    damage: u16,
    effect: Effect,
    used_signature: bool,
}

#[must_use]
#[allow(clippy::similar_names, clippy::too_many_lines)]
pub fn simulate_combat(match_: &Match, seed: u64) -> MatchResult {
    let mut rng = rng_new(seed);
    let max_hp_a = max_hp(&match_.loadout_a);
    let max_hp_b = max_hp(&match_.loadout_b);
    let mut state_a = CombatantState {
        hp: max_hp_a,
        stamina: starting_stamina(match_.loadout_a.pet_stats.end) as i32,
        ..CombatantState::default()
    };
    let mut state_b = CombatantState {
        hp: max_hp_b,
        stamina: starting_stamina(match_.loadout_b.pet_stats.end) as i32,
        ..CombatantState::default()
    };
    let mut rounds = [RoundLog::default(); MAX_ROUNDS];
    let mut round_count = 0_u8;

    for round in &mut rounds {
        let modifiers_a = begin_round(&mut state_a);
        let modifiers_b = begin_round(&mut state_b);
        if state_a.hp <= 0 || state_b.hp <= 0 {
            break;
        }

        let selected_cards = (
            select_card_ai(&match_.loadout_a, &state_a, &match_.loadout_b, &state_b),
            select_card_ai(&match_.loadout_b, &state_b, &match_.loadout_a, &state_a),
        );
        let card_a_idx = selected_cards.0;
        let card_b_idx = selected_cards.1;
        round.card_a_idx = card_a_idx;
        round.card_b_idx = card_b_idx;

        let initiative_a = initiative(&match_.loadout_a, card_a_idx, modifiers_a.slowed);
        let initiative_b = initiative(&match_.loadout_b, card_b_idx, modifiers_b.slowed);
        let a_first = if (initiative_a - initiative_b).abs() < f32::EPSILON {
            rng_unit_f32(&mut rng) < 0.5_f32
        } else {
            initiative_a > initiative_b
        };

        let (result_a, result_b) = if a_first {
            let a = act(
                &match_.loadout_a,
                card_a_idx,
                &mut state_a,
                max_hp_a,
                &match_.loadout_b,
                &mut state_b,
                modifiers_a.stunned,
                &mut rng,
            );
            let b = if state_b.hp > 0 {
                act(
                    &match_.loadout_b,
                    card_b_idx,
                    &mut state_b,
                    max_hp_b,
                    &match_.loadout_a,
                    &mut state_a,
                    modifiers_b.stunned,
                    &mut rng,
                )
            } else {
                ActionResult::default()
            };
            (a, b)
        } else {
            let b = act(
                &match_.loadout_b,
                card_b_idx,
                &mut state_b,
                max_hp_b,
                &match_.loadout_a,
                &mut state_a,
                modifiers_b.stunned,
                &mut rng,
            );
            let a = if state_a.hp > 0 {
                act(
                    &match_.loadout_a,
                    card_a_idx,
                    &mut state_a,
                    max_hp_a,
                    &match_.loadout_b,
                    &mut state_b,
                    modifiers_a.stunned,
                    &mut rng,
                )
            } else {
                ActionResult::default()
            };
            (a, b)
        };

        round.damage_a_to_b = result_a.damage;
        round.damage_b_to_a = result_b.damage;
        let effects_in_action_order = if a_first {
            [result_a.effect, result_b.effect]
        } else {
            [result_b.effect, result_a.effect]
        };
        round.effect_triggered = effects_in_action_order
            .into_iter()
            .find(|effect| effect.kind != EffectKind::None)
            .unwrap_or_default();
        finish_round(
            &mut state_a,
            match_.loadout_a.pet_stats.end,
            result_a.used_signature,
        );
        finish_round(
            &mut state_b,
            match_.loadout_b.pet_stats.end,
            result_b.used_signature,
        );
        round_count = round_count.saturating_add(1);

        if state_a.hp <= 0 || state_b.hp <= 0 {
            break;
        }
    }

    MatchResult {
        winner: determine_winner(state_a.hp, state_b.hp),
        rounds,
        round_count,
        final_hp_a: hp_for_result(state_a.hp),
        final_hp_b: hp_for_result(state_b.hp),
        seed,
    }
}

#[allow(clippy::too_many_arguments)]
fn act(
    attacker_loadout: &Loadout,
    card_idx: u8,
    attacker_state: &mut CombatantState,
    attacker_max_hp: i32,
    defender_loadout: &Loadout,
    defender_state: &mut CombatantState,
    stunned: bool,
    rng: &mut Rng,
) -> ActionResult {
    if stunned {
        return ActionResult::default();
    }

    let index = usize::from(card_idx.min(4));
    let card = &attacker_loadout.cards[index];
    let stamina_factor = if attacker_state.stamina < i32::from(card.stamina_cost) {
        attacker_state.stamina = 0;
        0.5_f32
    } else {
        attacker_state.stamina -= i32::from(card.stamina_cost);
        1.0_f32
    };
    let variance = rng_variance(rng, 0.9, 1.1);
    let crit_roll = rng_unit_f32(rng);
    let is_crit = crit_roll < finite_or_zero(card.crit_chance).clamp(0.0, 0.35);
    let crit_multiplier = if is_crit {
        if card.effect.kind == EffectKind::Crit && card.effect.value.is_finite() {
            card.effect.value.max(1.0)
        } else {
            1.8
        }
    } else {
        1.0
    };
    let damage = finite_or_zero(card.base_damage).max(0.0)
        * element_multiplier(
            attacker_loadout.pet_genome.element,
            defender_loadout.pet_genome.element,
        )
        * (1.0_f32 + tech_card_bonus(card.type_, attacker_loadout.pet_genome.tech_affinity))
        * mood_multiplier(attacker_loadout.pet_mood)
        * variance
        * (1.0_f32
            - defense_ratio(
                defender_loadout.pet_stats.foc,
                defender_loadout.gear.foc_bonus,
            ))
        * stamina_factor
        * crit_multiplier;
    let damage_i32 = rounded_nonnegative_i32(damage);
    defender_state.hp = defender_state.hp.saturating_sub(damage_i32);

    let effect = apply_card_effect(
        card.effect,
        is_crit,
        attacker_state,
        attacker_max_hp,
        defender_state,
    );
    ActionResult {
        damage: u16::try_from(damage_i32).unwrap_or(u16::MAX),
        effect,
        used_signature: index == usize::from(attacker_loadout.signature_idx.min(4)),
    }
}

fn apply_card_effect(
    effect: Effect,
    is_crit: bool,
    attacker: &mut CombatantState,
    attacker_max_hp: i32,
    defender: &mut CombatantState,
) -> Effect {
    if !effect.value.is_finite() || effect.value <= 0.0 {
        return Effect::default();
    }

    match effect.kind {
        EffectKind::None => Effect::default(),
        EffectKind::Stun => {
            defender.active.stun_rounds = defender
                .active
                .stun_rounds
                .saturating_add(effect.value.ceil().clamp(1.0, f32::from(u8::MAX)) as u8);
            effect
        }
        EffectKind::Bleed => {
            defender.active.bleed_stacks = defender
                .active
                .bleed_stacks
                .saturating_add(effect.value.ceil().clamp(1.0, f32::from(u8::MAX)) as u8);
            effect
        }
        EffectKind::Crit => {
            if is_crit {
                effect
            } else {
                Effect::default()
            }
        }
        EffectKind::Slow => {
            defender.active.slow_rounds = defender
                .active
                .slow_rounds
                .saturating_add(effect.value.ceil().clamp(1.0, f32::from(u8::MAX)) as u8);
            effect
        }
        EffectKind::Heal => {
            let healing = rounded_nonnegative_i32(effect.value);
            attacker.hp = attacker.hp.saturating_add(healing).min(attacker_max_hp);
            effect
        }
    }
}

fn begin_round(state: &mut CombatantState) -> TurnModifiers {
    let bleed_damage = i32::from(state.active.bleed_stacks) * BLEED_DAMAGE_PER_STACK;
    state.hp = state.hp.saturating_sub(bleed_damage);

    let stunned = state.active.stun_rounds > 0;
    state.active.stun_rounds = state.active.stun_rounds.saturating_sub(1);
    let slowed = state.active.slow_rounds > 0;
    state.active.slow_rounds = state.active.slow_rounds.saturating_sub(1);
    TurnModifiers { stunned, slowed }
}

fn finish_round(state: &mut CombatantState, end_stat: u32, used_signature: bool) {
    state.stamina = state.stamina.saturating_add(stamina_regen(end_stat) as i32);
    state.signature_cd = if used_signature {
        SIGNATURE_COOLDOWN_ROUNDS
    } else {
        state.signature_cd.saturating_sub(1)
    };
}

fn initiative(loadout: &Loadout, card_idx: u8, slowed: bool) -> f32 {
    let base = loadout.pet_stats.agi as f32
        + finite_or_zero(loadout.cards[usize::from(card_idx.min(4))].speed);
    if slowed {
        base - SLOW_INITIATIVE_PENALTY
    } else {
        base
    }
}

fn max_hp(loadout: &Loadout) -> i32 {
    let value =
        1_000_i64 + i64::from(loadout.pet_stats.end) * 10 + i64::from(loadout.gear.end_bonus) * 10;
    i32::try_from(value.clamp(1, i64::from(i32::MAX))).unwrap_or(i32::MAX)
}

fn rounded_nonnegative_i32(value: f32) -> i32 {
    finite_or_zero(value).round().clamp(0.0, i32::MAX as f32) as i32
}

fn hp_for_result(hp: i32) -> u16 {
    u16::try_from(hp.max(0)).unwrap_or(u16::MAX)
}

const fn determine_winner(hp_a: i32, hp_b: i32) -> Winner {
    if hp_b <= 0 && hp_a > hp_b {
        Winner::A
    } else if hp_a <= 0 && hp_b > hp_a {
        Winner::B
    } else if hp_a > hp_b {
        Winner::A
    } else if hp_b > hp_a {
        Winner::B
    } else {
        Winner::Draw
    }
}

#[must_use]
pub fn element_multiplier(attacker: Element, defender: Element) -> f32 {
    match (attacker.base_parents(), defender.base_parents()) {
        (None, None) => base_element_multiplier(attacker, defender),
        (Some((a, b)), None) => f32::midpoint(
            base_element_multiplier(a, defender),
            base_element_multiplier(b, defender),
        ),
        (None, Some((a, b))) => f32::midpoint(
            base_element_multiplier(attacker, a),
            base_element_multiplier(attacker, b),
        ),
        (Some((a1, a2)), Some((d1, d2))) => {
            (base_element_multiplier(a1, d1)
                + base_element_multiplier(a1, d2)
                + base_element_multiplier(a2, d1)
                + base_element_multiplier(a2, d2))
                / 4.0_f32
        }
    }
}

fn base_element_multiplier(attacker: Element, defender: Element) -> f32 {
    const TABLE: [[f32; 7]; 7] = [
        [1.0, 0.67, 1.5, 0.67, 0.91, 0.91, 1.1],
        [1.5, 1.0, 0.67, 1.5, 0.91, 0.91, 1.1],
        [0.67, 1.5, 1.0, 1.0, 0.91, 0.91, 1.1],
        [1.5, 0.67, 1.0, 1.0, 0.91, 0.91, 1.1],
        [1.1, 1.1, 1.1, 1.1, 1.0, 1.5, 0.91],
        [1.1, 1.1, 1.1, 1.1, 1.5, 1.0, 0.91],
        [0.91, 0.91, 0.91, 0.91, 1.1, 1.1, 1.0],
    ];
    let attacker_index = attacker as usize;
    let defender_index = defender as usize;
    debug_assert!(attacker_index < TABLE.len() && defender_index < TABLE.len());
    TABLE[attacker_index][defender_index]
}

#[must_use]
pub fn defense_ratio(foc_stat: u32, gear_foc_bonus: i16) -> f32 {
    (foc_stat as f32 + f32::from(gear_foc_bonus)).clamp(0.0, 200.0) / 400.0_f32
}

#[must_use]
pub const fn starting_stamina(end_stat: u32) -> u32 {
    100_u32.saturating_add(end_stat / 5)
}

#[must_use]
pub const fn stamina_regen(end_stat: u32) -> u32 {
    5_u32.saturating_add(end_stat / 50)
}

#[must_use]
pub fn rng_variance(rng: &mut Rng, lo: f32, hi: f32) -> f32 {
    if !lo.is_finite() || !hi.is_finite() || lo >= hi {
        return lo;
    }
    lo + (hi - lo) * rng_unit_f32(rng)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::technique::{Rarity, TechniqueType};

    fn card(damage: f32, speed: f32, stamina_cost: u16) -> TechniqueCard {
        TechniqueCard {
            type_: TechniqueType::Jab,
            element: Element::Fire,
            rarity: Rarity::Common,
            base_damage: damage,
            speed,
            stamina_cost,
            quality: 50,
            ..TechniqueCard::default()
        }
    }

    fn loadout(element: Element, damage: f32, speed: f32) -> Loadout {
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
            cards: [card(damage, speed, 10); 5],
            signature_idx: 4,
            ..Loadout::default()
        }
    }

    #[test]
    fn mvp_element_cycle_is_closed() {
        assert_eq!(element_multiplier(Element::Fire, Element::Earth), 1.5);
        assert_eq!(element_multiplier(Element::Earth, Element::Water), 1.5);
        assert_eq!(element_multiplier(Element::Water, Element::Fire), 1.5);
    }

    #[test]
    fn steam_averages_parent_multipliers() {
        assert!((element_multiplier(Element::Steam, Element::Earth) - 1.085).abs() < 0.000_1);
    }

    #[test]
    fn same_match_and_seed_produce_same_result() {
        let match_ = Match {
            loadout_a: loadout(Element::Fire, 120.0, 70.0),
            loadout_b: loadout(Element::Earth, 110.0, 60.0),
            mode: MatchMode::Casual,
        };
        assert_eq!(simulate_combat(&match_, 42), simulate_combat(&match_, 42));
    }

    #[test]
    fn combat_stops_at_round_limit() {
        let match_ = Match {
            loadout_a: loadout(Element::Fire, 1.0, 70.0),
            loadout_b: loadout(Element::Earth, 1.0, 60.0),
            mode: MatchMode::Casual,
        };
        assert_eq!(
            usize::from(simulate_combat(&match_, 1).round_count),
            MAX_ROUNDS
        );
    }

    #[test]
    fn observer_logs_first_effect_in_action_order() {
        let mut slower = loadout(Element::Fire, 1.0, 10.0);
        let mut faster = loadout(Element::Earth, 1.0, 100.0);
        for card in &mut slower.cards {
            card.effect = Effect {
                kind: EffectKind::Heal,
                value: 1.0,
            };
        }
        for card in &mut faster.cards {
            card.effect = Effect {
                kind: EffectKind::Slow,
                value: 1.0,
            };
        }
        let result = simulate_combat(
            &Match {
                loadout_a: slower,
                loadout_b: faster,
                mode: MatchMode::Casual,
            },
            9,
        );
        assert_eq!(result.rounds[0].effect_triggered.kind, EffectKind::Slow);
    }

    #[test]
    fn defense_is_capped_at_half() {
        assert_eq!(defense_ratio(100, 100), 0.5);
        assert_eq!(defense_ratio(100, 500), 0.5);
        assert_eq!(defense_ratio(0, -100), 0.0);
    }
}
