// Package fingerprint computes the short device fingerprint shown to users
// during the mandatory hard-stop verification prompt.
//
// Per the spec: SHA-256 of the public key string, truncated to the first 8
// hex characters, formatted as XXXX-XXXX (e.g. "7C91-42AE").
//
// The relay computes and returns this fingerprint alongside key lookups so
// the CLI never has to trust its own recomputation of a value the server
// could tamper with silently — both sides can compare independently.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Compute returns the formatted 8-hex-char fingerprint for a public key
// string (e.g. an age1... recipient key).
func Compute(publicKey string) string {
	sum := sha256.Sum256([]byte(publicKey))
	hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))[:8]
	return hexStr[:4] + "-" + hexStr[4:]
}
