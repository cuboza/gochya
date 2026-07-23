package dojo

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"
)

func TestPostgresStoreRequiresPool(t *testing.T) {
	if _, err := NewPostgresStore(PostgresStoreConfig{}); err == nil {
		t.Fatal("NewPostgresStore accepted a nil pool")
	}
}

func TestElementProtocolIDsMatchRustDiscriminants(t *testing.T) {
	names := []string{
		"Fire",
		"Water",
		"Earth",
		"Air",
		"Light",
		"Dark",
		"Arcane",
		"Steam",
		"Magma",
		"Storm",
		"Mud",
		"Smoke",
		"Sand",
		"Eclipse",
		"Inferno",
		"Prism",
		"Crystal",
	}
	for want, name := range names {
		got, ok := elementProtocolID(name)
		if !ok || got != uint8(want) {
			t.Fatalf("elementProtocolID(%q) = %d, %v; want %d, true", name, got, ok, want)
		}
	}
	if _, ok := elementProtocolID("unknown"); ok {
		t.Fatal("unknown element was accepted")
	}
}

func TestDecodeStoredResultRejectsCorruptHash(t *testing.T) {
	encoded, err := json.Marshal(SubmitResponse{EvidenceVerdict: "VALID"})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if _, _, err := decodeStoredResult(encoded, make([]byte, sha256.Size-1)); err == nil {
		t.Fatal("corrupt request hash was accepted")
	}
}

func TestNonceDigestIsStableAndDoesNotEqualBearerValue(t *testing.T) {
	value := "private-bearer-nonce"
	first := nonceDigest(value)
	second := nonceDigest(value)
	if first != second {
		t.Fatal("nonce digest is not deterministic")
	}
	if reflect.DeepEqual(first[:len(value)], []byte(value)) {
		t.Fatal("nonce digest contains the bearer value")
	}
}
