package crypto

import (
	"bytes"
	"fmt"

	"filippo.io/age"
)

// Encrypt encrypts a plaintext payload using age recipient-based encryption
// targeting the provided recipient public key string (e.g., "age1...").
func Encrypt(plaintext []byte, recipientPublicKey string) ([]byte, error) {
	recipient, err := age.ParseX25519Recipient(recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient public key: %w", err)
	}

	var out bytes.Buffer
	w, err := age.Encrypt(&out, recipient)
	if err != nil {
		return nil, fmt.Errorf("failed to create age encryptor: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("failed to write plaintext to age encryptor: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize age encryption: %w", err)
	}

	return out.Bytes(), nil
}
