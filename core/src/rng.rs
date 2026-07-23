use serde::{Deserialize, Serialize};

const PCG_MULTIPLIER: u64 = 6_364_136_223_846_793_005;
const PCG_STREAM: u64 = 1_442_695_040_888_963_407;

/// PCG-XSH-RR 64/32 with a fixed stream. The output is a zero-extended `u32`.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[repr(C)]
pub struct Rng {
    pub state: u64,
    pub inc: u64,
}

#[must_use]
pub fn rng_new(seed: u64) -> Rng {
    let mut rng = Rng {
        state: 0,
        inc: PCG_STREAM | 1,
    };
    let _ = rng_next(&mut rng);
    rng.state = rng.state.wrapping_add(seed);
    let _ = rng_next(&mut rng);
    rng
}

pub fn rng_next(rng: &mut Rng) -> u64 {
    let old_state = rng.state;
    rng.state = old_state
        .wrapping_mul(PCG_MULTIPLIER)
        .wrapping_add(rng.inc | 1);
    let xorshifted = (((old_state >> 18) ^ old_state) >> 27) as u32;
    let rotation = (old_state >> 59) as u32;
    u64::from(xorshifted.rotate_right(rotation))
}

/// Samples uniformly from the inclusive range `lo..=hi`.
///
/// Invalid or singleton ranges return `lo` without consuming the generator.
pub fn rng_range(rng: &mut Rng, lo: u32, hi: u32) -> u32 {
    if lo >= hi {
        return lo;
    }

    let bound = u64::from(hi) - u64::from(lo) + 1;
    let threshold = (1_u64 << 32) % bound;
    loop {
        let sample = rng_next(rng);
        if sample >= threshold {
            return lo + (sample % bound) as u32;
        }
    }
}

pub(crate) fn rng_unit_f32(rng: &mut Rng) -> f32 {
    let value = rng_next(rng) as u32 >> 8;
    value as f32 / 16_777_216.0_f32
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fixed_seed_has_stable_sequence() {
        let mut rng = rng_new(42);
        assert_eq!(
            [rng_next(&mut rng), rng_next(&mut rng), rng_next(&mut rng)],
            [3_270_867_926, 1_795_671_209, 1_924_641_435]
        );
    }

    #[test]
    fn range_is_inclusive() {
        let mut rng = rng_new(7);
        for _ in 0..1_000 {
            assert!((4..=24).contains(&rng_range(&mut rng, 4, 24)));
        }
    }

    #[test]
    fn invalid_range_is_safe() {
        let mut rng = rng_new(1);
        let before = rng;
        assert_eq!(rng_range(&mut rng, 9, 3), 9);
        assert_eq!(rng, before);
    }
}
