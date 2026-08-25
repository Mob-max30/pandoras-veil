package crypto

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// ComputeFingerprint calculates a short, deterministic, human-readable fingerprint
// from a public key string: SHA-256(pubKey) -> first 8 hex characters grouped as XXXX-XXXX (e.g. 7C91-42AE).
func ComputeFingerprint(publicKey string) string {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return "0000-0000"
	}
	hash := sha256.Sum256([]byte(publicKey))
	hexStr := strings.ToUpper(fmt.Sprintf("%x", hash))
	if len(hexStr) >= 8 {
		return fmt.Sprintf("%s-%s", hexStr[0:4], hexStr[4:8])
	}
	return hexStr
}
