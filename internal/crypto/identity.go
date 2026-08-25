package crypto

import (
	"fmt"
	"strings"

	"filippo.io/age"
)

// DeviceIdentity represents a local device's cryptographic identity
type DeviceIdentity struct {
	Identity    *age.X25519Identity
	PrivateKey  string
	PublicKey   string
	Fingerprint string
}

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
	secretKey = strings.TrimSpace(secretKey)
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

// ParseRecipient parses an age recipient public key string (age1...)
func ParseRecipient(pubKeyStr string) (age.Recipient, error) {
	pubKeyStr = strings.TrimSpace(pubKeyStr)
	recipient, err := age.ParseX25519Recipient(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient public key: %w", err)
	}
	return recipient, nil
}

// GenerateDeviceIdentity generates a DeviceIdentity struct for CLI usage
func GenerateDeviceIdentity() (*DeviceIdentity, error) {
	identity, err := GenerateIdentity()
	if err != nil {
		return nil, err
	}
	pubKey := GetPublicKey(identity)
	privKey := identity.String()
	fp := Fingerprint(pubKey)

	return &DeviceIdentity{
		Identity:    identity,
		PrivateKey:  privKey,
		PublicKey:   pubKey,
		Fingerprint: fp,
	}, nil
}
