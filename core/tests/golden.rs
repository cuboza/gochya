use gochya_core::{
    Element, Genome, Loadout, Match, MatchMode, Stats, TechniqueCard, TechniqueType, Winner,
    simulate_combat,
};
use serde::Deserialize;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct GoldenFixture {
    schema_version: u32,
    seed: u64,
    fighter_a: FighterFixture,
    fighter_b: FighterFixture,
    expected: ExpectedResult,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FighterFixture {
    element: Element,
    base_damage: f32,
    speed: f32,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExpectedResult {
    winner: Winner,
    round_count: u8,
    final_hp_a: u16,
    final_hp_b: u16,
    rounds: Vec<ExpectedRound>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExpectedRound {
    card_a_idx: u8,
    card_b_idx: u8,
    damage_a_to_b: u16,
    damage_b_to_a: u16,
}

#[test]
fn combat_matches_v1_golden_fixture() {
    let fixture: GoldenFixture =
        serde_json::from_str(include_str!("golden/combat_v1.json")).expect("valid fixture");
    assert_eq!(fixture.schema_version, 1);
    let result = simulate_combat(
        &Match {
            loadout_a: loadout(&fixture.fighter_a),
            loadout_b: loadout(&fixture.fighter_b),
            mode: MatchMode::Casual,
        },
        fixture.seed,
    );

    assert_eq!(result.winner, fixture.expected.winner);
    assert_eq!(result.round_count, fixture.expected.round_count);
    assert_eq!(result.final_hp_a, fixture.expected.final_hp_a);
    assert_eq!(result.final_hp_b, fixture.expected.final_hp_b);
    assert_eq!(
        fixture.expected.rounds.len(),
        usize::from(result.round_count)
    );
    for (actual, expected) in result.rounds.iter().zip(&fixture.expected.rounds) {
        assert_eq!(actual.card_a_idx, expected.card_a_idx);
        assert_eq!(actual.card_b_idx, expected.card_b_idx);
        assert_eq!(actual.damage_a_to_b, expected.damage_a_to_b);
        assert_eq!(actual.damage_b_to_a, expected.damage_b_to_a);
    }
}

fn loadout(fighter: &FighterFixture) -> Loadout {
    let card = TechniqueCard {
        type_: TechniqueType::Jab,
        element: fighter.element,
        base_damage: fighter.base_damage,
        speed: fighter.speed,
        stamina_cost: 10,
        quality: 50,
        ..TechniqueCard::default()
    };
    Loadout {
        pet_stats: Stats {
            str: 30,
            agi: 30,
            end: 30,
            foc: 30,
        },
        pet_genome: Genome {
            element: fighter.element,
            tech_affinity: TechniqueType::Jab,
            ..Genome::default()
        },
        pet_mood: 100,
        cards: [card; 5],
        signature_idx: 4,
        ..Loadout::default()
    }
}
