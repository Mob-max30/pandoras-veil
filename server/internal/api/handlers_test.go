package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Mob-max30/pandoras-veil/server/internal/idgen"
	"github.com/Mob-max30/pandoras-veil/server/internal/store"
)

// fakeStore is an in-memory stand-in for *store.Store so handler tests don't
// require a live Redis instance. It intentionally mirrors the atomicity
// guarantee GetPaste's real implementation provides for burn-after-reading.
type fakeStore struct {
	mu     sync.Mutex
	keys   map[string]store.KeyRecord
	pastes map[string]string
	down   bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		keys:   make(map[string]store.KeyRecord),
		pastes: make(map[string]string),
	}
}

func (f *fakeStore) RegisterKey(_ context.Context, handle, publicKey, fingerprint string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return store.ErrUnavailable
	}
	if _, exists := f.keys[handle]; exists {
		return store.ErrHandleTaken
	}
	f.keys[handle] = store.KeyRecord{Handle: handle, PublicKey: publicKey, Fingerprint: fingerprint}
	return nil
}

func (f *fakeStore) LookupKey(_ context.Context, handle string) (store.KeyRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return store.KeyRecord{}, store.ErrUnavailable
	}
	rec, ok := f.keys[handle]
	if !ok {
		return store.KeyRecord{}, store.ErrNotFound
	}
	return rec, nil
}

func (f *fakeStore) PutPaste(_ context.Context, id string, ciphertextB64 string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return store.ErrUnavailable
	}
	f.pastes[id] = ciphertextB64
	return nil
}

// GetPaste mirrors the real store's atomicity: burn IDs are deleted on the
// same locked pass that reads them, so a second concurrent call can never
// also observe the value.
func (f *fakeStore) GetPaste(_ context.Context, id string) (store.PasteRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return store.PasteRecord{}, store.ErrUnavailable
	}
	val, ok := f.pastes[id]
	if !ok {
		return store.PasteRecord{}, store.ErrNotFound
	}
	if idgen.IsBurnAfterReading(id) {
		delete(f.pastes, id)
	}
	return store.PasteRecord{Ciphertext: val}, nil
}

func (f *fakeStore) Ping(context.Context) error {
	if f.down {
		return store.ErrUnavailable
	}
	return nil
}

func (f *fakeStore) Subscribe(_ context.Context, _ string) *redis.PubSub {
	return nil
}

func (f *fakeStore) Publish(_ context.Context, _, _ string) error {
	return nil
}

func newTestHandlers(fs *fakeStore) *Handlers {
	return New(fs, TTLPolicy{Default: 15 * time.Minute, Min: 30 * time.Second, Max: 7 * 24 * time.Hour}, 2*1024*1024, slog.Default())
}

func doRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRegisterKey_GeneratesHandleAndFingerprint(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	rec := doRequest(t, router, "POST", "/keys", registerKeyRequest{PublicKey: "age1exampletestkey"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp registerKeyResponse
	mustDecode(t, rec, &resp)
	if resp.Handle == "" {
		t.Error("expected a generated handle, got empty string")
	}
	if resp.Fingerprint == "" {
		t.Error("expected a fingerprint, got empty string")
	}
}

func TestRegisterKey_ConflictOnDuplicateHandle(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	first := doRequest(t, router, "POST", "/keys", registerKeyRequest{Handle: "alice", PublicKey: "age1aaa"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first register status = %d, want 201", first.Code)
	}

	second := doRequest(t, router, "POST", "/keys", registerKeyRequest{Handle: "alice", PublicKey: "age1bbb"})
	if second.Code != http.StatusConflict {
		t.Fatalf("second register status = %d, want 409", second.Code)
	}
}

func TestLookupKey_NotFound(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	rec := doRequest(t, router, "GET", "/keys/nobody", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestLookupKey_ReturnsRegisteredKey(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	doRequest(t, router, "POST", "/keys", registerKeyRequest{Handle: "bob", PublicKey: "age1bobkey"})

	rec := doRequest(t, router, "GET", "/keys/bob", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp lookupKeyResponse
	mustDecode(t, rec, &resp)
	if resp.PublicKey != "age1bobkey" {
		t.Errorf("public_key = %q, want %q", resp.PublicKey, "age1bobkey")
	}
}

func TestPasteRoundTrip_PersistentTTL(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))
	ciphertext := base64.StdEncoding.EncodeToString([]byte("totally-encrypted-bytes"))

	uploadRec := doRequest(t, router, "POST", "/paste", uploadPasteRequest{Ciphertext: ciphertext, BurnAfterReading: false})
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body=%s", uploadRec.Code, uploadRec.Body.String())
	}
	var uploadResp uploadPasteResponse
	mustDecode(t, uploadRec, &uploadResp)

	// Non-burn pastes must survive multiple reads.
	for i := 0; i < 2; i++ {
		fetchRec := doRequest(t, router, "GET", "/paste/"+uploadResp.ID, nil)
		if fetchRec.Code != http.StatusOK {
			t.Fatalf("fetch #%d status = %d, want 200", i, fetchRec.Code)
		}
		var fetchResp fetchPasteResponse
		mustDecode(t, fetchRec, &fetchResp)
		if fetchResp.Ciphertext != ciphertext {
			t.Errorf("fetch #%d ciphertext mismatch", i)
		}
	}
}

func TestPasteRoundTrip_BurnAfterReading(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))
	ciphertext := base64.StdEncoding.EncodeToString([]byte("burn-me-once"))

	uploadRec := doRequest(t, router, "POST", "/paste", uploadPasteRequest{Ciphertext: ciphertext, BurnAfterReading: true})
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201", uploadRec.Code)
	}
	var uploadResp uploadPasteResponse
	mustDecode(t, uploadRec, &uploadResp)

	firstRead := doRequest(t, router, "GET", "/paste/"+uploadResp.ID, nil)
	if firstRead.Code != http.StatusOK {
		t.Fatalf("first read status = %d, want 200", firstRead.Code)
	}

	secondRead := doRequest(t, router, "GET", "/paste/"+uploadResp.ID, nil)
	if secondRead.Code != http.StatusNotFound {
		t.Fatalf("second read status = %d, want 404 (burned)", secondRead.Code)
	}
}

func TestFetchPaste_UnknownID(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	rec := doRequest(t, router, "GET", "/paste/does-not-exist", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUploadPaste_RejectsInvalidBase64(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	rec := doRequest(t, router, "POST", "/paste", map[string]any{
		"ciphertext":         "not-valid-base64!!!",
		"burn_after_reading": false,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUploadPaste_RejectsOversizedCiphertext(t *testing.T) {
	fs := newFakeStore()
	h := New(fs, TTLPolicy{Default: time.Minute, Min: time.Second, Max: time.Hour}, 8 /* tiny cap */, slog.Default())
	router := NewRouter(h)

	oversized := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 100))
	rec := doRequest(t, router, "POST", "/paste", uploadPasteRequest{Ciphertext: oversized})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHealth_ReflectsStoreAvailability(t *testing.T) {
	fs := newFakeStore()
	router := NewRouter(newTestHandlers(fs))

	ok := doRequest(t, router, "GET", "/health", nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("healthy status = %d, want 200", ok.Code)
	}

	fs.mu.Lock()
	fs.down = true
	fs.mu.Unlock()

	degraded := doRequest(t, router, "GET", "/health", nil)
	if degraded.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded status = %d, want 503", degraded.Code)
	}
}

func TestHandleStream_MissingHandle(t *testing.T) {
	router := NewRouter(newTestHandlers(newFakeStore()))

	rec := doRequest(t, router, "GET", "/stream", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func mustDecode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
}
