use gochya_core::{Element, combat::element_multiplier};

const EPSILON: f32 = 0.000_01;

#[test]
fn element_balance_holds_for_every_release_set() {
    assert_balanced(&[Element::Fire, Element::Water, Element::Earth]);
    assert_balanced(&[
        Element::Fire,
        Element::Water,
        Element::Earth,
        Element::Steam,
    ]);
    assert_balanced(&[Element::Fire, Element::Water, Element::Earth, Element::Air]);
    assert_balanced(&[
        Element::Fire,
        Element::Water,
        Element::Earth,
        Element::Air,
        Element::Light,
        Element::Dark,
        Element::Arcane,
    ]);
}

fn assert_balanced(elements: &[Element]) {
    for &element in elements {
        let has_counter = elements
            .iter()
            .copied()
            .any(|attacker| element_multiplier(attacker, element) > 1.0 + EPSILON);
        assert!(has_counter, "{element:?} has no counter in {elements:?}");

        let row_weakly_dominates_column = elements.iter().copied().all(|opponent| {
            element_multiplier(element, opponent) + EPSILON >= element_multiplier(opponent, element)
        });
        let has_strict_advantage = elements.iter().copied().any(|opponent| {
            element_multiplier(element, opponent) > element_multiplier(opponent, element) + EPSILON
        });
        assert!(
            !(row_weakly_dominates_column && has_strict_advantage),
            "{element:?} weakly dominates its release set"
        );
    }
}
