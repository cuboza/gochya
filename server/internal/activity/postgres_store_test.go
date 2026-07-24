package activity

import "testing"

func TestApplyStatGainsReconcilesAndClamps(t *testing.T) {
	stats, applied, deltas := applyStatGains(
		petStats{Strength: 10, Agility: 20, Endurance: 30, Focus: 2},
		StatGains{},
		StatGains{Strength: 7, Agility: 7, Endurance: 12, Focus: -5},
	)
	if stats != (petStats{Strength: 17, Agility: 27, Endurance: 42}) ||
		applied != (StatGains{Strength: 7, Agility: 7, Endurance: 12, Focus: -2}) ||
		deltas != (StatDeltas{Strength: 7, Agility: 7, Endurance: 12, Focus: -2}) {
		t.Fatalf("first reconciliation = %#v / %#v / %#v", stats, applied, deltas)
	}

	stats, applied, deltas = applyStatGains(
		stats,
		applied,
		StatGains{Strength: 3, Agility: 4, Endurance: 5, Focus: 0},
	)
	if stats != (petStats{Strength: 13, Agility: 24, Endurance: 35, Focus: 2}) ||
		applied != (StatGains{Strength: 3, Agility: 4, Endurance: 5}) ||
		deltas != (StatDeltas{Strength: -4, Agility: -3, Endurance: -7, Focus: 2}) {
		t.Fatalf("corrected reconciliation = %#v / %#v / %#v", stats, applied, deltas)
	}
}
