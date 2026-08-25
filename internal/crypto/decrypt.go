package crypto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"filippo.io/age"
)

var (
	// ErrDecryptionFailed is returned when decryption fails due to an unauthorized key or tampered payload.
	ErrDecryptionFailed = errors.New("this secret could not be decrypted (it may not be addressed to this device, or it may have been tampered with)")
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

	// Try base64 decoding if ciphertext starts with base64 age header
	raw := ciphertext
	if decoded, err := base64.StdEncoding.DecodeString(string(ciphertext)); err == nil && len(decoded) > 0 {
		raw = decoded
	}

	r, err := age.Decrypt(bytes.NewReader(raw), identity)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}
