use serde::{Deserialize, Serialize};

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
    pub tech_affinity: crate::technique::TechniqueType,
    pub rarity: crate::technique::Rarity,
    pub ability: Ability,
    pub generation: u32,
}
