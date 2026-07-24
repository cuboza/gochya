use serde::{Deserialize, Serialize};

use crate::{
    rng::{Rng, rng_new, rng_range, rng_unit_f32},
    technique::{Rarity, TechniqueType},
};

/// Base and hybrid creature elements. Discriminants are stable protocol IDs.
#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum Element {
    #[default]
    Fire = 0,
    Water = 1,
    Earth = 2,
    Air = 3,
    Light = 4,
    Dark = 5,
    Arcane = 6,
    Steam = 7,
    Magma = 8,
    Storm = 9,
    Mud = 10,
    Smoke = 11,
    Sand = 12,
    Eclipse = 13,
    Inferno = 14,
    Prism = 15,
    Crystal = 16,
}

impl Element {
    #[must_use]
    pub const fn base_parents(self) -> Option<(Self, Self)> {
        match self {
            Self::Steam => Some((Self::Fire, Self::Water)),
            Self::Magma => Some((Self::Fire, Self::Earth)),
            Self::Storm => Some((Self::Air, Self::Water)),
            Self::Mud => Some((Self::Earth, Self::Water)),
            Self::Smoke => Some((Self::Fire, Self::Air)),
            Self::Sand => Some((Self::Earth, Self::Air)),
            Self::Eclipse => Some((Self::Light, Self::Dark)),
            Self::Inferno => Some((Self::Fire, Self::Dark)),
            Self::Prism => Some((Self::Water, Self::Light)),
            Self::Crystal => Some((Self::Earth, Self::Light)),
            _ => None,
        }
    }

    #[must_use]
    pub const fn is_base(self) -> bool {
        self.base_parents().is_none()
    }
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum Ability {
    #[default]
    None = 0,
    Regen = 1,
    CritAura = 2,
    Thorns = 3,
    Shield = 4,
    Lifesteal = 5,
    LineageSignature = 6,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct VisualGenes {
    pub body_shape: u8,
    pub palette_hue: u16,
    pub palette_sat: u8,
    pub pattern: u8,
    pub size: u8,
    pub eye_style: u8,
    pub aura: u8,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct StatPotentials {
    pub str_pot: u8,
    pub agi_pot: u8,
    pub end_pot: u8,
    pub foc_pot: u8,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct Genome {
    pub visual: VisualGenes,
    pub stats: StatPotentials,
    pub element: Element,
    pub tech_affinity: TechniqueType,
    pub rarity: Rarity,
    pub ability: Ability,
    pub generation: u32,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct Catalysts {
    pub mutation: bool,
    pub hybrid: bool,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct BreedResult {
    pub genome: Genome,
    pub incubation_hours: u8,
    pub mutated_genes: u16,
}

const MUTATED_BODY_SHAPE: u16 = 1 << 0;
const MUTATED_PALETTE_HUE: u16 = 1 << 1;
const MUTATED_PALETTE_SAT: u16 = 1 << 2;
const MUTATED_PATTERN: u16 = 1 << 3;
const MUTATED_SIZE: u16 = 1 << 4;
const MUTATED_EYE_STYLE: u16 = 1 << 5;
const MUTATED_AURA: u16 = 1 << 6;
const MUTATED_STR_POT: u16 = 1 << 7;
const MUTATED_AGI_POT: u16 = 1 << 8;
const MUTATED_END_POT: u16 = 1 << 9;
const MUTATED_FOC_POT: u16 = 1 << 10;
const MUTATED_ELEMENT: u16 = 1 << 11;
const MUTATED_TECH_AFFINITY: u16 = 1 << 12;
const MUTATED_ABILITY: u16 = 1 << 13;

/// Produces a deterministic offspring genome from two authoritative parents.
///
/// The inbreeding coefficient is supplied by the server because lineage lives
/// in persistent storage rather than the pure gameplay core.
#[must_use]
pub fn breed(
    parent_a: &Genome,
    parent_b: &Genome,
    catalysts: &Catalysts,
    inbreeding_coeff: u8,
    seed: u64,
) -> BreedResult {
    let mut rng = rng_new(seed);
    let incubation_hours = rng_range(&mut rng, 4, 24) as u8;
    let mut genome = inherited_genome(&mut rng, parent_a, parent_b);

    if parent_a.element != parent_b.element
        && rng_unit_f32(&mut rng) < hybrid_chance(*catalysts)
        && let Some(hybrid) = hybrid_of(parent_a.element, parent_b.element)
        && hybrid == Element::Steam
    {
        genome.element = hybrid;
    }

    let chance = mutation_chance(parent_a, parent_b, catalysts, inbreeding_coeff);
    let mutated_genes = apply_mutations(&mut rng, chance, &mut genome);

    BreedResult {
        genome,
        incubation_hours,
        mutated_genes,
    }
}

fn inherited_genome(rng: &mut Rng, parent_a: &Genome, parent_b: &Genome) -> Genome {
    Genome {
        visual: VisualGenes {
            body_shape: inherit(rng, parent_a.visual.body_shape, parent_b.visual.body_shape),
            palette_hue: inherit(
                rng,
                parent_a.visual.palette_hue,
                parent_b.visual.palette_hue,
            ),
            palette_sat: inherit(
                rng,
                parent_a.visual.palette_sat,
                parent_b.visual.palette_sat,
            ),
            pattern: inherit(rng, parent_a.visual.pattern, parent_b.visual.pattern),
            size: inherit(rng, parent_a.visual.size, parent_b.visual.size),
            eye_style: inherit(rng, parent_a.visual.eye_style, parent_b.visual.eye_style),
            aura: inherit(rng, parent_a.visual.aura, parent_b.visual.aura),
        },
        stats: StatPotentials {
            str_pot: inherit_stat(rng, parent_a.stats.str_pot, parent_b.stats.str_pot),
            agi_pot: inherit_stat(rng, parent_a.stats.agi_pot, parent_b.stats.agi_pot),
            end_pot: inherit_stat(rng, parent_a.stats.end_pot, parent_b.stats.end_pot),
            foc_pot: inherit_stat(rng, parent_a.stats.foc_pot, parent_b.stats.foc_pot),
        },
        element: inherit(rng, parent_a.element, parent_b.element),
        tech_affinity: inherit(rng, parent_a.tech_affinity, parent_b.tech_affinity),
        rarity: inherit(rng, parent_a.rarity, parent_b.rarity),
        ability: inherit(rng, parent_a.ability, parent_b.ability),
        generation: parent_a
            .generation
            .max(parent_b.generation)
            .saturating_add(1),
    }
}

fn apply_mutations(rng: &mut Rng, chance: f32, genome: &mut Genome) -> u16 {
    let mut mutated_genes = 0_u16;
    mutate_visual_genes(rng, chance, &mut genome.visual, &mut mutated_genes);
    mutate_stat_genes(rng, chance, &mut genome.stats, &mut mutated_genes);
    if rng_unit_f32(rng) < chance {
        genome.element = mutate_element(rng, genome.element);
        mutated_genes |= MUTATED_ELEMENT;
    }
    if rng_unit_f32(rng) < chance {
        genome.tech_affinity = mutate_technique(rng, genome.tech_affinity);
        mutated_genes |= MUTATED_TECH_AFFINITY;
    }
    if rng_unit_f32(rng) < chance {
        genome.ability = mutate_ability(rng, genome.ability);
        mutated_genes |= MUTATED_ABILITY;
    }
    mutated_genes
}

fn mutate_visual_genes(
    rng: &mut Rng,
    chance: f32,
    visual: &mut VisualGenes,
    mutated_genes: &mut u16,
) {
    mutate_gene(
        rng,
        chance,
        &mut visual.body_shape,
        7,
        MUTATED_BODY_SHAPE,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut visual.palette_hue,
        360,
        MUTATED_PALETTE_HUE,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut visual.palette_sat,
        100,
        MUTATED_PALETTE_SAT,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut visual.pattern,
        7,
        MUTATED_PATTERN,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut visual.size,
        7,
        MUTATED_SIZE,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut visual.eye_style,
        7,
        MUTATED_EYE_STYLE,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut visual.aura,
        7,
        MUTATED_AURA,
        mutated_genes,
    );
}

fn mutate_stat_genes(
    rng: &mut Rng,
    chance: f32,
    stats: &mut StatPotentials,
    mutated_genes: &mut u16,
) {
    mutate_gene(
        rng,
        chance,
        &mut stats.str_pot,
        100,
        MUTATED_STR_POT,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut stats.agi_pot,
        100,
        MUTATED_AGI_POT,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut stats.end_pot,
        100,
        MUTATED_END_POT,
        mutated_genes,
    );
    mutate_gene(
        rng,
        chance,
        &mut stats.foc_pot,
        100,
        MUTATED_FOC_POT,
        mutated_genes,
    );
}

#[must_use]
pub fn mutation_chance(
    parent_a: &Genome,
    parent_b: &Genome,
    catalysts: &Catalysts,
    inbreeding_coeff: u8,
) -> f32 {
    let average_rarity = f32::midpoint(
        f32::from(parent_a.rarity as u8),
        f32::from(parent_b.rarity as u8),
    );
    (0.04
        + 0.01 * average_rarity
        + if parent_a.element == parent_b.element {
            0.0
        } else {
            0.05
        }
        + if catalysts.mutation { 0.10 } else { 0.0 }
        - 0.02 * f32::from(inbreeding_coeff))
    .clamp(0.0, 0.30)
}

#[must_use]
pub fn stat_cap_penalty(generation: u32) -> f32 {
    generation.saturating_sub(5) as f32 * 0.03
}

#[must_use]
pub const fn hybrid_of(first: Element, second: Element) -> Option<Element> {
    match (first, second) {
        (Element::Fire, Element::Water) | (Element::Water, Element::Fire) => Some(Element::Steam),
        (Element::Fire, Element::Earth) | (Element::Earth, Element::Fire) => Some(Element::Magma),
        (Element::Air, Element::Water) | (Element::Water, Element::Air) => Some(Element::Storm),
        (Element::Earth, Element::Water) | (Element::Water, Element::Earth) => Some(Element::Mud),
        (Element::Fire, Element::Air) | (Element::Air, Element::Fire) => Some(Element::Smoke),
        (Element::Earth, Element::Air) | (Element::Air, Element::Earth) => Some(Element::Sand),
        (Element::Light, Element::Dark) | (Element::Dark, Element::Light) => Some(Element::Eclipse),
        (Element::Fire, Element::Dark) | (Element::Dark, Element::Fire) => Some(Element::Inferno),
        (Element::Water, Element::Light) | (Element::Light, Element::Water) => Some(Element::Prism),
        (Element::Earth, Element::Light) | (Element::Light, Element::Earth) => {
            Some(Element::Crystal)
        }
        _ => None,
    }
}

fn hybrid_chance(catalysts: Catalysts) -> f32 {
    (0.20_f32 + if catalysts.hybrid { 0.15 } else { 0.0 }).min(0.50)
}

fn inherit<T: Copy>(rng: &mut Rng, first: T, second: T) -> T {
    if rng_range(rng, 0, 1) == 0 {
        first
    } else {
        second
    }
}

fn inherit_stat(rng: &mut Rng, first: u8, second: u8) -> u8 {
    let lower = u32::from(first.min(second)) * 95 / 100;
    let upper = (u32::from(first.max(second)) * 105).div_ceil(100).min(100);
    rng_range(rng, lower, upper) as u8
}

trait Gene: Copy + Eq {
    fn from_roll(value: u32) -> Self;
    fn to_roll(self) -> u32;
}

impl Gene for u8 {
    fn from_roll(value: u32) -> Self {
        value as Self
    }

    fn to_roll(self) -> u32 {
        u32::from(self)
    }
}

impl Gene for u16 {
    fn from_roll(value: u32) -> Self {
        value as Self
    }

    fn to_roll(self) -> u32 {
        u32::from(self)
    }
}

fn mutate_gene<T: Gene>(
    rng: &mut Rng,
    chance: f32,
    gene: &mut T,
    maximum: u32,
    bit: u16,
    mutated_genes: &mut u16,
) {
    if rng_unit_f32(rng) >= chance || maximum == 0 {
        return;
    }
    let current = gene.to_roll().min(maximum);
    let roll = rng_range(rng, 0, maximum - 1);
    let replacement = if roll >= current { roll + 1 } else { roll };
    *gene = T::from_roll(replacement);
    *mutated_genes |= bit;
}

fn mutate_element(rng: &mut Rng, current: Element) -> Element {
    let current_roll = match current {
        Element::Fire => Some(0),
        Element::Water => Some(1),
        Element::Earth => Some(2),
        _ => None,
    };
    let roll = match current_roll {
        Some(current_roll) => {
            let roll = rng_range(rng, 0, 1) as u8;
            if roll >= current_roll { roll + 1 } else { roll }
        }
        None => rng_range(rng, 0, 2) as u8,
    };
    match roll {
        0 => Element::Fire,
        1 => Element::Water,
        _ => Element::Earth,
    }
}

fn mutate_technique(rng: &mut Rng, current: TechniqueType) -> TechniqueType {
    let current = current as u8;
    let roll = rng_range(rng, 0, 5) as u8;
    match if roll >= current { roll + 1 } else { roll } {
        0 => TechniqueType::Jab,
        1 => TechniqueType::Hook,
        2 => TechniqueType::Uppercut,
        3 => TechniqueType::Cross,
        4 => TechniqueType::Kick,
        5 => TechniqueType::Elbow,
        _ => TechniqueType::Block,
    }
}

fn mutate_ability(rng: &mut Rng, current: Ability) -> Ability {
    let current = current as u8;
    let roll = rng_range(rng, 0, 5) as u8;
    match if roll >= current { roll + 1 } else { roll } {
        0 => Ability::None,
        1 => Ability::Regen,
        2 => Ability::CritAura,
        3 => Ability::Thorns,
        4 => Ability::Shield,
        5 => Ability::Lifesteal,
        _ => Ability::LineageSignature,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn genome(element: Element, rarity: Rarity, generation: u32, offset: u8) -> Genome {
        Genome {
            visual: VisualGenes {
                body_shape: offset,
                palette_hue: 30 + u16::from(offset),
                palette_sat: 60 + offset,
                pattern: offset,
                size: offset,
                eye_style: offset,
                aura: offset,
            },
            stats: StatPotentials {
                str_pot: 50 + offset,
                agi_pot: 60 + offset,
                end_pot: 70 + offset,
                foc_pot: 80 + offset,
            },
            element,
            tech_affinity: TechniqueType::Hook,
            rarity,
            ability: Ability::Regen,
            generation,
        }
    }

    #[test]
    fn breeding_is_deterministic_and_bounded() {
        let first = genome(Element::Fire, Rarity::Rare, 2, 1);
        let second = genome(Element::Water, Rarity::Epic, 5, 2);
        let catalysts = Catalysts {
            mutation: true,
            hybrid: true,
        };
        let result = breed(&first, &second, &catalysts, 0, 42);
        assert_eq!(result, breed(&first, &second, &catalysts, 0, 42));
        assert!((4..=24).contains(&result.incubation_hours));
        assert_eq!(result.genome.generation, 6);
        assert!(result.genome.visual.palette_hue <= 360);
        assert!(result.genome.visual.palette_sat <= 100);
        assert!(result.genome.stats.str_pot <= 100);
        assert!(result.genome.stats.agi_pot <= 100);
        assert!(result.genome.stats.end_pot <= 100);
        assert!(result.genome.stats.foc_pot <= 100);
        assert_eq!(result.mutated_genes & !0x3fff, 0);
    }

    #[test]
    fn mutation_formula_and_generation_penalty_match_specification() {
        let first = genome(Element::Fire, Rarity::Rare, 0, 0);
        let second = genome(Element::Water, Rarity::Epic, 0, 0);
        let chance = mutation_chance(
            &first,
            &second,
            &Catalysts {
                mutation: true,
                hybrid: false,
            },
            2,
        );
        assert!((chance - 0.175).abs() < f32::EPSILON);
        assert_eq!(stat_cap_penalty(5), 0.0);
        assert!((stat_cap_penalty(8) - 0.09).abs() < f32::EPSILON);
    }

    #[test]
    fn hybrid_table_is_order_independent() {
        assert_eq!(
            hybrid_of(Element::Fire, Element::Water),
            Some(Element::Steam)
        );
        assert_eq!(
            hybrid_of(Element::Water, Element::Fire),
            Some(Element::Steam)
        );
        assert_eq!(hybrid_of(Element::Fire, Element::Arcane), None);
    }

    #[test]
    fn current_breeding_release_never_emits_unreleased_hybrids() {
        let fire = genome(Element::Fire, Rarity::Common, 0, 0);
        let earth = genome(Element::Earth, Rarity::Common, 0, 0);
        let catalysts = Catalysts {
            mutation: false,
            hybrid: true,
        };
        for seed in 0..1_000 {
            let result = breed(&fire, &earth, &catalysts, 0, seed);
            assert_ne!(result.genome.element, Element::Magma);
            assert_ne!(result.genome.element, Element::Mud);
        }
    }
}
