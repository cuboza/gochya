package corebridge

import "testing"

func TestValidateABIVersion(t *testing.T) {
	for _, version := range []uint32{0x0002_0300, 0x0002_0401, 0x0002_ffff} {
		if err := validateABIVersion(version); err != nil {
			t.Fatalf("compatible ABI 0x%08x: %v", version, err)
		}
	}
	for _, version := range []uint32{0x0001_ffff, 0x0002_0200, 0x0003_0000} {
		if err := validateABIVersion(version); err == nil {
			t.Fatalf("incompatible ABI 0x%08x was accepted", version)
		}
	}
}
