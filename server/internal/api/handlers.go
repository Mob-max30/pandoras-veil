package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Mob-max30/pandoras-veil/server/internal/fingerprint"
	"github.com/Mob-max30/pandoras-veil/server/internal/idgen"
	"github.com/Mob-max30/pandoras-veil/server/internal/store"
)

// Store is the subset of *store.Store the handlers depend on. Defining it
// as an interface here (rather than importing the concrete type directly
// into every signature) keeps handler tests able to swap in a fake without
// touching Redis.
type Store interface {
	RegisterKey(ctx context.Context, handle, publicKey, fingerprint string) error
	LookupKey(ctx context.Context, handle string) (store.KeyRecord, error)
	DeleteKey(ctx context.Context, handle string) error
	PutPaste(ctx context.Context, id string, ciphertextB64 string, ttl time.Duration) error
	GetPaste(ctx context.Context, id string) (store.PasteRecord, error)
	Ping(ctx context.Context) error
	Subscribe(ctx context.Context, channel string) *redis.PubSub
	Publish(ctx context.Context, channel, message string) error
	PushInboxMessage(ctx context.Context, recipient, sender, msgJSON string, ttl time.Duration) error
	GetAndClearInbox(ctx context.Context, recipient, sender string) ([]string, error)
	GetAllAndClearInbox(ctx context.Context, recipient string) ([]string, error)
	FlushDB(ctx context.Context) error
}

// TTLPolicy clamps a caller-requested TTL to the server's configured bounds.
type TTLPolicy struct {
	Default time.Duration
	Min     time.Duration
	Max     time.Duration
}

func (p TTLPolicy) resolve(requestedSeconds *int) time.Duration {
	if requestedSeconds == nil {
		return p.Default
	}
	d := time.Duration(*requestedSeconds) * time.Second
	if d < p.Min {
		return p.Min
	}
	if d > p.Max {
		return p.Max
	}
	return d
}

// Handlers holds dependencies shared across route handlers.
type Handlers struct {
	Store              Store
	TTL                TTLPolicy
	MaxCiphertextBytes int64
	Logger             *slog.Logger
}

// New constructs a Handlers with the given dependencies.
func New(s Store, ttl TTLPolicy, maxCiphertextBytes int64, logger *slog.Logger) *Handlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{Store: s, TTL: ttl, MaxCiphertextBytes: maxCiphertextBytes, Logger: logger}
}

// ---- POST /keys ---------------------------------------------------------

func (h *Handlers) RegisterKey(w http.ResponseWriter, r *http.Request) {
	var req registerKeyRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "public_key is required")
		return
	}

	handle := req.Handle
	if handle == "" {
		var err error
		handle, err = randomHandle()
		if err != nil {
			h.Logger.Error("generating handle", "error", err)
			writeError(w, http.StatusInternalServerError, "could not generate handle")
			return
		}
	}

	fp := fingerprint.Compute(req.PublicKey)

	err := h.Store.RegisterKey(r.Context(), handle, req.PublicKey, fp)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, registerKeyResponse{Handle: handle, Fingerprint: fp})
	case errors.Is(err, store.ErrHandleTaken):
		writeError(w, http.StatusConflict, "handle already registered")
	default:
		h.Logger.Error("registering key", "error", err, "handle", handle)
		writeError(w, http.StatusServiceUnavailable, "relay storage unavailable")
	}
}

// ---- GET /keys/{handle} ---------------------------------------------------

func (h *Handlers) LookupKey(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	if handle == "" {
		writeError(w, http.StatusBadRequest, "handle is required")
		return
	}

	rec, err := h.Store.LookupKey(r.Context(), handle)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, lookupKeyResponse{PublicKey: rec.PublicKey, Fingerprint: rec.Fingerprint})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "handle not found")
	default:
		h.Logger.Error("looking up key", "error", err, "handle", handle)
		writeError(w, http.StatusServiceUnavailable, "relay storage unavailable")
	}
}

// DeleteKey handles DELETE /keys/{handle}
func (h *Handlers) DeleteKey(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	if handle == "" {
		writeError(w, http.StatusBadRequest, "handle is required")
		return
	}

	if err := h.Store.DeleteKey(r.Context(), handle); err != nil {
		h.Logger.Error("deleting key", "error", err, "handle", handle)
		writeError(w, http.StatusInternalServerError, "failed to delete key")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "handle": handle})
}

// ---- POST /paste ----------------------------------------------------------

func (h *Handlers) UploadPaste(w http.ResponseWriter, r *http.Request) {
	var req uploadPasteRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Ciphertext == "" {
		writeError(w, http.StatusBadRequest, "ciphertext is required")
		return
	}

	// Validate base64 and enforce the size ceiling on decoded bytes, without
	// keeping the decoded copy around — the relay stores the base64 string
	// verbatim and never needs the raw bytes itself.
	if _, err := base64.StdEncoding.DecodeString(req.Ciphertext); err != nil {
		writeError(w, http.StatusBadRequest, "ciphertext must be valid base64")
		return
	}
	if maxLen := base64.StdEncoding.DecodedLen(len(req.Ciphertext)); int64(maxLen) > h.MaxCiphertextBytes {
		writeError(w, http.StatusBadRequest, "ciphertext exceeds maximum allowed size")
		return
	}

	ttl := h.TTL.resolve(req.TTLSeconds)

	id, err := idgen.New(req.BurnAfterReading)
	if err != nil {
		h.Logger.Error("generating paste id", "error", err)
		writeError(w, http.StatusInternalServerError, "could not generate share id")
		return
	}

	if err := h.Store.PutPaste(r.Context(), id, req.Ciphertext, ttl); err != nil {
		h.Logger.Error("storing paste", "error", err, "id", id)
		writeError(w, http.StatusServiceUnavailable, "relay storage unavailable")
		return
	}

	// Queue in recipient's inbox and publish to Redis Pub/Sub for real-time recipients
	if req.Recipient != "" {
		eventPayload, err := json.Marshal(map[string]string{
			"id":         id,
			"ciphertext": req.Ciphertext,
			"sender":     req.Sender,
		})
		if err == nil {
			if req.Sender != "" {
				_ = h.Store.PushInboxMessage(r.Context(), req.Recipient, req.Sender, string(eventPayload), ttl)
			}
			_ = h.Store.Publish(r.Context(), "stream:"+req.Recipient, string(eventPayload))
		}
	}

	writeJSON(w, http.StatusCreated, uploadPasteResponse{ID: id})
}

// ---- GET /paste/{id} --------------------------------------------------------

func (h *Handlers) FetchPaste(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	rec, err := h.Store.GetPaste(r.Context(), id)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, fetchPasteResponse{Ciphertext: rec.Ciphertext})
	case errors.Is(err, store.ErrNotFound):
		// Covers: never existed, TTL-expired, or already burned by an
		// earlier read. The client can't and shouldn't distinguish these.
		writeError(w, http.StatusNotFound, "paste not found or expired")
	default:
		h.Logger.Error("fetching paste", "error", err, "id", id)
		writeError(w, http.StatusServiceUnavailable, "relay storage unavailable")
	}
}

// ---- GET /stream?handle={handle}&with={sender} ---------------------------

func (h *Handlers) HandleStream(w http.ResponseWriter, r *http.Request) {
	handle := r.URL.Query().Get("handle")
	if handle == "" {
		writeError(w, http.StatusBadRequest, "handle query parameter is required")
		return
	}

	withParam := r.URL.Query().Get("with")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Flush pending queued inbox messages immediately upon establishing connection
	if withParam != "" {
		queuedMsgs, err := h.Store.GetAndClearInbox(r.Context(), handle, withParam)
		if err == nil {
			for _, qm := range queuedMsgs {
				fmt.Fprintf(w, "data: %s\n\n", qm)
				flusher.Flush()
			}
		}
	} else {
		queuedMsgs, err := h.Store.GetAllAndClearInbox(r.Context(), handle)
		if err == nil {
			for _, qm := range queuedMsgs {
				fmt.Fprintf(w, "data: %s\n\n", qm)
				flusher.Flush()
			}
		}
	}

	// Subscribe to Redis channel: stream:<handle>
	pubsub := h.Store.Subscribe(r.Context(), "stream:"+handle)
	if pubsub == nil {
		return
	}
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// If `with` filter is set, filter out live messages from other senders
			if withParam != "" {
				var evt struct {
					Sender string `json:"sender"`
				}
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err == nil && evt.Sender != "" {
					if !strings.EqualFold(evt.Sender, withParam) {
						continue
					}
					// Clear from inbox list if it was queued during stream latency
					_, _ = h.Store.GetAndClearInbox(r.Context(), handle, withParam)
				}
			}
			fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flusher.Flush()
		}
	}
}

// ---- GET /inbox?recipient={recipient}&sender={sender} ---------------------

func (h *Handlers) FetchInbox(w http.ResponseWriter, r *http.Request) {
	recipient := r.URL.Query().Get("recipient")
	sender := r.URL.Query().Get("sender")
	if recipient == "" {
		writeError(w, http.StatusBadRequest, "recipient query parameter is required")
		return
	}

	var msgsRaw []string
	var err error
	if sender != "" {
		msgsRaw, err = h.Store.GetAndClearInbox(r.Context(), recipient, sender)
	} else {
		msgsRaw, err = h.Store.GetAllAndClearInbox(r.Context(), recipient)
	}

	if err != nil {
		h.Logger.Error("fetching inbox", "error", err, "recipient", recipient, "sender", sender)
		writeError(w, http.StatusServiceUnavailable, "relay storage unavailable")
		return
	}

	var msgs []inboxMessage
	for _, raw := range msgsRaw {
		var msg inboxMessage
		if err := json.Unmarshal([]byte(raw), &msg); err == nil {
			msgs = append(msgs, msg)
		}
	}
	if msgs == nil {
		msgs = []inboxMessage{}
	}

	writeJSON(w, http.StatusOK, fetchInboxResponse{Messages: msgs})
}

// ---- POST/DELETE /admin/clear ----------------------------------------------

func (h *Handlers) FlushServer(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.FlushDB(r.Context()); err != nil {
		h.Logger.Error("flushing server db", "error", err)
		writeError(w, http.StatusServiceUnavailable, "failed to flush database")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

// ---- GET /health ------------------------------------------------------------

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// ---- shared helpers -----------------------------------------------------

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func randomHandle() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "veil-" + hex.EncodeToString(buf), nil
}
