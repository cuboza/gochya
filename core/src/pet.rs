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

#[cfg(test)]
mod tests {
    use super::mood_multiplier;

    #[test]
    fn mood_multiplier_is_bounded() {
        assert_eq!(mood_multiplier(0), 0.7);
        assert_eq!(mood_multiplier(100), 1.0);
        assert_eq!(mood_multiplier(u8::MAX), 1.0);
    }
}
