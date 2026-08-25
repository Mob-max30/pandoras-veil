package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Fingerprint calculates a short, deterministic identity-verification fingerprint
// for a given recipient public key string (e.g., "age1...").
//
// ENCODING SPECIFICATION:
// 1. Hash algorithm: SHA-256 computed over the UTF-8 bytes of the public key string.
// 2. Truncation: First 4 bytes (32 bits) of the 32-byte digest.
// 3. Format: Uppercase hexadecimal string (8 hex characters).
// 4. Grouping: 2 blocks of 4 hex characters separated by a hyphen (e.g., "7C91-42AE").
//
// Note: The fingerprint is an out-of-band identity-verification aid, NOT a password or secret.
func Fingerprint(publicKey string) string {
	if publicKey == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(publicKey))
	// Take first 4 bytes (8 hex characters)
	hexStr := strings.ToUpper(hex.EncodeToString(hash[:4]))
	if len(hexStr) != 8 {
		return ""
	}
	return fmt.Sprintf("%s-%s", hexStr[:4], hexStr[4:])
}
