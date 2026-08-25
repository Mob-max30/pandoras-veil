package crypto

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"filippo.io/age"
)

// Encrypt encrypts plaintext bytes targeting the specified recipient public key string (age1...)
// and returns base64 encoded ciphertext bytes.
func Encrypt(plaintext []byte, recipientPubKey string) (string, error) {
	recipient, err := ParseRecipient(recipientPubKey)
	if err != nil {
		return "", fmt.Errorf("recipient key parse error: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return "", fmt.Errorf("age encryption initialization failed: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return "", fmt.Errorf("failed to write plaintext to age encryptor: %w", err)
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize age encryption: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
