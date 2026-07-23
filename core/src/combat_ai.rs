use crate::{
    combat::{CombatantState, Loadout, defense_ratio, element_multiplier},
    technique::{finite_or_zero, tech_card_bonus},
};

/// Selects the most stamina-efficient card. Ties are resolved by stable index.
#[must_use]
pub fn select_card_ai(
    my_loadout: &Loadout,
    my_state: &CombatantState,
    enemy_loadout: &Loadout,
    enemy_state: &CombatantState,
) -> u8 {
    let mut best_index = 0_u8;
    let mut best_score = f32::NEG_INFINITY;

    for (index, card) in my_loadout.cards.iter().enumerate() {
        if index == usize::from(my_loadout.signature_idx.min(4)) && my_state.signature_cd > 0 {
            continue;
        }

        let stamina_factor = if my_state.stamina < i32::from(card.stamina_cost) {
            0.5_f32
        } else {
            1.0_f32
        };
        let expected_damage = finite_or_zero(card.base_damage).max(0.0)
            * element_multiplier(
                my_loadout.pet_genome.element,
                enemy_loadout.pet_genome.element,
            )
            * (1.0_f32 + tech_card_bonus(card.type_, my_loadout.pet_genome.tech_affinity))
            * (1.0_f32 - defense_ratio(enemy_loadout.pet_stats.foc, enemy_loadout.gear.foc_bonus))
            * stamina_factor;
        let lethal_bonus = if expected_damage >= enemy_state.hp.max(0) as f32 {
            1.5_f32
        } else {
            1.0_f32
        };
        let score = expected_damage * lethal_bonus / f32::from(card.stamina_cost.max(1));

        if score > best_score {
            best_score = score;
            best_index = u8::try_from(index).unwrap_or(0);
        }
    }

    best_index
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{combat::CombatantState, technique::TechniqueCard};

    #[test]
    fn chooses_damage_per_stamina_and_uses_stable_tie_break() {
        let mut loadout = Loadout::default();
        loadout.cards[0] = TechniqueCard {
            base_damage: 20.0,
            stamina_cost: 10,
            ..TechniqueCard::default()
        };
        loadout.cards[1] = TechniqueCard {
            base_damage: 30.0,
            stamina_cost: 10,
            ..TechniqueCard::default()
        };
        assert_eq!(
            select_card_ai(
                &loadout,
                &CombatantState::default(),
                &Loadout::default(),
                &CombatantState {
                    hp: 1_000,
                    ..CombatantState::default()
                }
            ),
            1
        );
    }

    #[test]
    fn skips_signature_during_cooldown() {
        let mut loadout = Loadout {
            signature_idx: 1,
            ..Loadout::default()
        };
        loadout.cards[1].base_damage = 1_000.0;
        let state = CombatantState {
            stamina: 100,
            signature_cd: 3,
            ..CombatantState::default()
        };
        assert_ne!(
            select_card_ai(
                &loadout,
                &state,
                &Loadout::default(),
                &CombatantState {
                    hp: 1_000,
                    ..CombatantState::default()
                }
            ),
            1
        );
    }
}
