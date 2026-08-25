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

// GenerateIdentity creates a new X25519 identity for the device
func GenerateIdentity() (*DeviceIdentity, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate X25519 identity: %w", err)
	}

	pubKey := identity.Recipient().String()
	privKey := identity.String()
	fingerprint := ComputeFingerprint(pubKey)

	return &DeviceIdentity{
		Identity:    identity,
		PrivateKey:  privKey,
		PublicKey:   pubKey,
		Fingerprint: fingerprint,
	}, nil
}

// ParseIdentity parses an age secret key string (AGE-SECRET-KEY-1...) into a DeviceIdentity
func ParseIdentity(privKeyStr string) (*DeviceIdentity, error) {
	privKeyStr = strings.TrimSpace(privKeyStr)
	identity, err := age.ParseX25519Identity(privKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	pubKey := identity.Recipient().String()
	fingerprint := ComputeFingerprint(pubKey)

	return &DeviceIdentity{
		Identity:    identity,
		PrivateKey:  privKeyStr,
		PublicKey:   pubKey,
		Fingerprint: fingerprint,
	}, nil
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
