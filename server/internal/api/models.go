// Package api implements the four frozen relay routes plus /health.
//
// Owner: Pavan (Backend Relay Lead) — server/
//
// IMPORTANT: These request/response shapes are the API contract frozen at
// Coordination Point 1. Do not change field names or types without
// reopening that sync with Pranav (crypto/CLI encode/decode side) and
// Ujwal (CLI/TUI consumer side) per the README's mandatory instructions.
package api

// --- POST /keys ----------------------------------------------------------

type registerKeyRequest struct {
	Handle    string `json:"handle,omitempty"`
	PublicKey string `json:"public_key"`
}

type registerKeyResponse struct {
	Handle      string `json:"handle"`
	Fingerprint string `json:"fingerprint"`
}

// --- GET /keys/:handle -----------------------------------------------------

type lookupKeyResponse struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// --- POST /paste -----------------------------------------------------------

type uploadPasteRequest struct {
	Ciphertext       string `json:"ciphertext"` // base64-encoded bytes
	Recipient        string `json:"recipient,omitempty"`
	Sender           string `json:"sender,omitempty"`
	TTLSeconds       *int   `json:"ttl_seconds,omitempty"`
	BurnAfterReading bool   `json:"burn_after_reading"`
}

type uploadPasteResponse struct {
	ID string `json:"id"`
}

// --- GET /paste/:id ---------------------------------------------------------

type fetchPasteResponse struct {
	Ciphertext string `json:"ciphertext"` // base64-encoded bytes
}

// --- GET /health -------------------------------------------------------------

type healthResponse struct {
	Status string `json:"status"`
}

// --- GET /inbox --------------------------------------------------------------

type inboxMessage struct {
	ID         string `json:"id"`
	Ciphertext string `json:"ciphertext"`
	Sender     string `json:"sender"`
}

type fetchInboxResponse struct {
	Messages []inboxMessage `json:"messages"`
}

// --- shared error shape --------------------------------------------------

// errorResponse is the single error shape used across every route, per the
// "error shapes" item frozen at Coordination Point 1.
type errorResponse struct {
	Error string `json:"error"`
}
