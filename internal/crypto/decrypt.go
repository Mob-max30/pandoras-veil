package crypto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"

	"filippo.io/age"
)

// ErrDecryptionFailed is the standardized generic decryption error
var ErrDecryptionFailed = errors.New("this secret could not be decrypted (it may not be addressed to this device, or it may have been tampered with)")

// Decrypt decrypts base64-encoded ciphertext using the provided local identity.
// If decryption fails due to key mismatch or tampering, it returns ErrDecryptionFailed.
func Decrypt(ciphertextB64 string, identity *age.X25519Identity) ([]byte, error) {
	rawCiphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		// Also try raw bytes in case it was passed directly
		rawCiphertext = []byte(ciphertextB64)
	}

	r, err := age.Decrypt(bytes.NewReader(rawCiphertext), identity)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		return nil, ErrDecryptionFailed
	}

	return out.Bytes(), nil
}
