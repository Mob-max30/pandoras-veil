package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"filippo.io/age"
)

var (
	// ErrDecryptionFailed is returned when decryption fails due to an unauthorized key or tampered payload.
	ErrDecryptionFailed = errors.New("decryption failed: invalid key or tampered payload")
)

// Decrypt decrypts age-encrypted ciphertext bytes using the recipient's local X25519 identity.
// Returns a generic error if key is wrong or ciphertext has been tampered with.
func Decrypt(ciphertext []byte, identity *age.X25519Identity) ([]byte, error) {
	if identity == nil {
		return nil, errors.New("identity cannot be nil")
	}

	if len(ciphertext) == 0 {
		return nil, ErrDecryptionFailed
	}

	r, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}
