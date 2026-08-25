package crypto_test

import (
	"regexp"
	"testing"

	"github.com/Mob-max30/pandoras-veil/internal/crypto"
)

func TestFingerprintDeterministic(t *testing.T) {
	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	pubKey := crypto.GetPublicKey(id)
	fp1 := crypto.Fingerprint(pubKey)
	fp2 := crypto.Fingerprint(pubKey)

	if fp1 == "" {
		t.Fatal("fingerprint should not be empty")
	}
	if fp1 != fp2 {
		t.Errorf("fingerprint is not deterministic: got %s and %s", fp1, fp2)
	}
}

func TestFingerprintFormat(t *testing.T) {
	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	pubKey := crypto.GetPublicKey(id)
	fp := crypto.Fingerprint(pubKey)

	// Format must be XXXX-XXXX (8 uppercase hex characters divided by a hyphen)
	matched, err := regexp.MatchString(`^[0-9A-F]{4}-[0-9A-F]{4}$`, fp)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Errorf("fingerprint %s does not match expected format XXXX-XXXX", fp)
	}
}

func TestFingerprintUniqueness(t *testing.T) {
	id1, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}
	id2, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	pk1 := crypto.GetPublicKey(id1)
	pk2 := crypto.GetPublicKey(id2)

	fp1 := crypto.Fingerprint(pk1)
	fp2 := crypto.Fingerprint(pk2)

	if fp1 == fp2 {
		t.Errorf("expected different fingerprints for different keys (%s and %s), got both = %s", pk1, pk2, fp1)
	}
}

func TestFingerprintEmpty(t *testing.T) {
	fp := crypto.Fingerprint("")
	if fp != "" {
		t.Errorf("expected empty fingerprint for empty public key, got %s", fp)
	}
}
