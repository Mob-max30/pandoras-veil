package crypto

import (
	"fmt"

	"filippo.io/age"
)

// GenerateIdentity creates a new X25519 device identity using filippo.io/age.
// The private key must remain local to the device and never be transmitted.
func GenerateIdentity() (*age.X25519Identity, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate age identity: %w", err)
	}
	return identity, nil
}

// ParseIdentity parses a Bech32-encoded age secret key string (AGE-SECRET-KEY-1...)
// back into an age.X25519Identity.
func ParseIdentity(secretKey string) (*age.X25519Identity, error) {
	identity, err := age.ParseX25519Identity(secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse age identity: %w", err)
	}
	return identity, nil
}

// GetPublicKey derives the recipient (public key) Bech32 string (age1...)
// corresponding to the given age identity.
func GetPublicKey(identity *age.X25519Identity) string {
	if identity == nil {
		return ""
	}
	return identity.Recipient().String()
}
