package crypto_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Mob-max30/pandoras-veil/internal/crypto"
)

// TEST 1: Authorized device can decrypt.
func TestAuthorizedDeviceDecrypts(t *testing.T) {
	// Generate identities for Device A (sender) and Device B (recipient)
	_, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device A identity: %v", err)
	}

	deviceB, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device B identity: %v", err)
	}

	pubKeyB := crypto.GetPublicKey(deviceB)
	originalPlaintext := []byte("Super secret payload bound to Device B")

	// Device A encrypts for Device B
	ciphertext, err := crypto.Encrypt(originalPlaintext, pubKeyB)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("expected non-empty ciphertext")
	}

	// Device B decrypts
	decryptedPlaintext, err := crypto.Decrypt(ciphertext, deviceB)
	if err != nil {
		t.Fatalf("Decrypt failed for authorized device B: %v", err)
	}

	if !bytes.Equal(decryptedPlaintext, originalPlaintext) {
		t.Errorf("decrypted content mismatch: expected %s, got %s", string(originalPlaintext), string(decryptedPlaintext))
	}
}

// TEST 2: Unauthorized device C CANNOT decrypt.
func TestUnauthorizedDeviceCannotDecrypt(t *testing.T) {
	deviceB, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device B identity: %v", err)
	}

	deviceC, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device C identity: %v", err)
	}

	pubKeyB := crypto.GetPublicKey(deviceB)
	originalPlaintext := []byte("Top secret meant exclusively for Device B")

	// Encrypt for Device B
	ciphertext, err := crypto.Encrypt(originalPlaintext, pubKeyB)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Device C (unauthorized) attempts to decrypt
	decrypted, err := crypto.Decrypt(ciphertext, deviceC)
	if err == nil {
		t.Fatalf("unauthorized Device C successfully decrypted ciphertext! Output: %s", string(decrypted))
	}

	if !errors.Is(err, crypto.ErrDecryptionFailed) {
		t.Errorf("expected ErrDecryptionFailed, got %v", err)
	}

	if len(decrypted) > 0 {
		t.Errorf("expected empty plaintext on failure, got %d bytes", len(decrypted))
	}
}

// TEST 3: Tampered ciphertext fails.
func TestTamperedCiphertextFails(t *testing.T) {
	deviceB, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device B identity: %v", err)
	}

	pubKeyB := crypto.GetPublicKey(deviceB)
	originalPlaintext := []byte("Tamper protection test payload")

	ciphertext, err := crypto.Encrypt(originalPlaintext, pubKeyB)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Modify bytes in different positions (header, payload body, MAC tag)
	tamperPositions := []int{
		0,
		len(ciphertext) / 2,
		len(ciphertext) - 1,
	}

	for _, pos := range tamperPositions {
		tamperedCiphertext := make([]byte, len(ciphertext))
		copy(tamperedCiphertext, ciphertext)
		tamperedCiphertext[pos] ^= 0xFF // Flip bits

		decrypted, err := crypto.Decrypt(tamperedCiphertext, deviceB)
		if err == nil {
			t.Fatalf("decryption succeeded on tampered ciphertext at index %d! Output: %s", pos, string(decrypted))
		}

		if !errors.Is(err, crypto.ErrDecryptionFailed) {
			t.Errorf("expected ErrDecryptionFailed for tamper at index %d, got %v", pos, err)
		}
	}
}

// TEST 4: Empty plaintext should be handled consistently.
func TestEmptyPlaintextRoundtrip(t *testing.T) {
	deviceB, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device B identity: %v", err)
	}

	pubKeyB := crypto.GetPublicKey(deviceB)
	emptyPlaintext := []byte("")

	ciphertext, err := crypto.Encrypt(emptyPlaintext, pubKeyB)
	if err != nil {
		t.Fatalf("Encrypt failed for empty plaintext: %v", err)
	}

	decrypted, err := crypto.Decrypt(ciphertext, deviceB)
	if err != nil {
		t.Fatalf("Decrypt failed for empty plaintext: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("expected 0 bytes decrypted plaintext, got %d bytes", len(decrypted))
	}
}

// TEST 5: Round-trip multiple plaintext values (normal, multiline, large).
func TestMultiplePlaintextRoundtrips(t *testing.T) {
	deviceB, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate Device B identity: %v", err)
	}

	pubKeyB := crypto.GetPublicKey(deviceB)

	// Reasonably sized payload (16 KB)
	largeText := strings.Repeat("Pandora's Veil Device-Bound Zero-Knowledge Secret Relay\n", 300)

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "Normal text",
			plaintext: []byte("Hello, Pandora's Veil secret sharing!"),
		},
		{
			name:      "Multiline text",
			plaintext: []byte("Line 1: Secret Key\nLine 2: Confidential Data\nLine 3: 🔒 End of Secret"),
		},
		{
			name:      "16KB text payload",
			plaintext: []byte(largeText),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := crypto.Encrypt(tc.plaintext, pubKeyB)
			if err != nil {
				t.Fatalf("Encrypt failed for '%s': %v", tc.name, err)
			}

			decrypted, err := crypto.Decrypt(ciphertext, deviceB)
			if err != nil {
				t.Fatalf("Decrypt failed for '%s': %v", tc.name, err)
			}

			if !bytes.Equal(decrypted, tc.plaintext) {
				t.Errorf("mismatch for '%s'", tc.name)
			}
		})
	}
}

func TestEncryptInvalidPublicKey(t *testing.T) {
	_, err := crypto.Encrypt([]byte("test"), "invalid-pubkey")
	if err == nil {
		t.Error("expected error when encrypting for invalid public key, got nil")
	}
}
