package crypto_test

import (
	"strings"
	"testing"

	"github.com/Mob-max30/pandoras-veil/internal/crypto"
)

func TestGenerateIdentity(t *testing.T) {
	identity, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}
	if identity == nil {
		t.Fatal("expected non-nil identity")
	}

	secretKey := identity.String()
	if !strings.HasPrefix(secretKey, "AGE-SECRET-KEY-1") {
		t.Errorf("expected secret key to start with 'AGE-SECRET-KEY-1', got %s", secretKey)
	}

	pubKey := crypto.GetPublicKey(identity)
	if !strings.HasPrefix(pubKey, "age1") {
		t.Errorf("expected public key to start with 'age1', got %s", pubKey)
	}
}

func TestParseIdentity(t *testing.T) {
	original, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	secretKey := original.String()
	parsed, err := crypto.ParseIdentity(secretKey)
	if err != nil {
		t.Fatalf("ParseIdentity failed: %v", err)
	}

	if parsed.String() != original.String() {
		t.Errorf("secret key mismatch: expected %s, got %s", original.String(), parsed.String())
	}

	originalPubKey := crypto.GetPublicKey(original)
	parsedPubKey := crypto.GetPublicKey(parsed)
	if originalPubKey != parsedPubKey {
		t.Errorf("public key mismatch: expected %s, got %s", originalPubKey, parsedPubKey)
	}
}

func TestParseIdentityInvalid(t *testing.T) {
	_, err := crypto.ParseIdentity("invalid-secret-key")
	if err == nil {
		t.Error("expected error when parsing invalid secret key, got nil")
	}
}

func TestGetPublicKeyNil(t *testing.T) {
	pubKey := crypto.GetPublicKey(nil)
	if pubKey != "" {
		t.Errorf("expected empty string for nil identity, got %s", pubKey)
	}
}
