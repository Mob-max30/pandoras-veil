// Package idgen generates share IDs for uploaded pastes.
//
// IDs are random, not sequential or content-derived — the share ID's only
// job is to locate ciphertext on the relay. It grants zero decryption
// authority on its own (that's the whole point of the device-bound model),
// so we don't need it to be unguessable-forever, just collision-resistant
// and awkward to enumerate.
//
// The ID also self-describes its read semantics (burn-after-reading vs.
// TTL-only) via a one-character prefix. This matters because
// GET /paste/:id carries no request body per the API contract — the
// handler must know, from the ID alone, whether to issue an atomic
// Redis GETDEL or a plain GET. Storing the mode as a second field
// alongside the ciphertext would require a read-then-conditional-delete,
// which is not atomic and reintroduces the double-read race
// burn-after-reading exists to prevent. Encoding it in the ID keeps the
// single Redis round trip atomic and keeps the store dependency-free.
package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

// randomLength is the number of random bytes used per ID before encoding.
// 15 bytes -> 24 base32 chars, giving 120 bits of entropy.
const randomLength = 15

const (
	// PrefixBurn marks an ID as burn-after-reading (atomic GETDEL on read).
	PrefixBurn = 'b'
	// PrefixPersist marks an ID as TTL-only (plain GET on read, expires
	// naturally via Redis TTL).
	PrefixPersist = 'p'
)

// New returns a new URL-safe, lowercase share ID whose first character
// encodes whether the paste is burn-after-reading.
func New(burnAfterReading bool) (string, error) {
	buf := make([]byte, randomLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	enc := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))

	prefix := PrefixPersist
	if burnAfterReading {
		prefix = PrefixBurn
	}
	return fmt.Sprintf("%c%s", prefix, enc), nil
}

// IsBurnAfterReading reports whether id was generated with burn-after-reading
// semantics. Malformed or empty IDs are treated as non-burn (safe default:
// falls through to a plain GET, which just returns not-found for a bogus ID).
func IsBurnAfterReading(id string) bool {
	return len(id) > 0 && id[0] == PrefixBurn
}
