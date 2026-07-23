use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct HeartRateEvidence {
    pub baseline: u16,
    pub mean: u16,
    pub present: f32,
    pub confidence: f32,
    pub delta: i16,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[repr(u8)]
pub enum HeartFailReason {
    #[default]
    Ok = 0,
    LowPresence = 1,
    NoElevation = 2,
    TooLow = 3,
    PoorContact = 4,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[repr(C)]
pub struct HeartVerdict {
    pub passed: bool,
    pub heart_score: f32,
    pub reason: HeartFailReason,
}

#[must_use]
pub fn validate_heart(evidence: &HeartRateEvidence) -> HeartVerdict {
    let reason = if !evidence.present.is_finite() || evidence.present < 0.80_f32 {
        HeartFailReason::LowPresence
    } else if u32::from(evidence.mean) < u32::from(evidence.baseline) + 8 {
        HeartFailReason::NoElevation
    } else if evidence.mean < 55 {
        HeartFailReason::TooLow
    } else if !evidence.confidence.is_finite() || evidence.confidence < 0.85_f32 {
        HeartFailReason::PoorContact
    } else {
        HeartFailReason::Ok
    };

    let passed = reason == HeartFailReason::Ok;
    HeartVerdict {
        passed,
        heart_score: if passed { heart_score(evidence) } else { 0.0 },
        reason,
    }
}

#[must_use]
pub fn spirit_bonus(evidence: &HeartRateEvidence) -> f32 {
    ((f32::from(evidence.delta) - 8.0_f32) / 40.0_f32).clamp(0.0, 0.20)
}

#[must_use]
pub fn heart_score(evidence: &HeartRateEvidence) -> f32 {
    if validate_gate_only(evidence) {
        0.5_f32 + spirit_bonus(evidence)
    } else {
        0.0
    }
}

fn validate_gate_only(evidence: &HeartRateEvidence) -> bool {
    evidence.present.is_finite()
        && evidence.present >= 0.80_f32
        && u32::from(evidence.mean) >= u32::from(evidence.baseline) + 8
        && evidence.mean >= 55
        && evidence.confidence.is_finite()
        && evidence.confidence >= 0.85_f32
}

#[cfg(test)]
mod tests {
    use super::*;

    fn valid_evidence() -> HeartRateEvidence {
        HeartRateEvidence {
            baseline: 70,
            mean: 90,
            present: 0.9,
            confidence: 0.9,
            delta: 20,
        }
    }

    #[test]
    fn valid_evidence_passes() {
        let verdict = validate_heart(&valid_evidence());
        assert!(verdict.passed);
        assert_eq!(verdict.reason, HeartFailReason::Ok);
        assert!((verdict.heart_score - 0.70).abs() < f32::EPSILON);
    }

    #[test]
    fn absent_heart_has_zero_score() {
        let evidence = HeartRateEvidence {
            present: 0.0,
            ..valid_evidence()
        };
        let verdict = validate_heart(&evidence);
        assert!(!verdict.passed);
        assert_eq!(verdict.reason, HeartFailReason::LowPresence);
        assert_eq!(verdict.heart_score, 0.0);
    }

    #[test]
    fn non_finite_contact_values_fail_closed() {
        let evidence = HeartRateEvidence {
            confidence: f32::NAN,
            ..valid_evidence()
        };
        assert_eq!(
            validate_heart(&evidence).reason,
            HeartFailReason::PoorContact
        );
    }
}
