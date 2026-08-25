package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

type mockRelayClient struct {
	client.RelayClient
	keys map[string]*client.KeyInfo
}

func (m *mockRelayClient) Health() error {
	return nil
}

func (m *mockRelayClient) RegisterKey(handle, publicKey string) (*client.KeyInfo, error) {
	k := &client.KeyInfo{
		Handle:      handle,
		PublicKey:   publicKey,
		Fingerprint: crypto.ComputeFingerprint(publicKey),
	}
	m.keys[handle] = k
	return k, nil
}

func (m *mockRelayClient) GetKey(handle string) (*client.KeyInfo, error) {
	if k, ok := m.keys[handle]; ok {
		return k, nil
	}
	id, _ := crypto.GenerateIdentity()
	pub := id.Recipient().String()
	return &client.KeyInfo{
		Handle:      handle,
		PublicKey:   pub,
		Fingerprint: crypto.ComputeFingerprint(pub),
	}, nil
}

func (m *mockRelayClient) PostPaste(ciphertext string, ttlSeconds int, burnAfterReading bool) (string, error) {
	return "pv_12345", nil
}

func (m *mockRelayClient) PostChatMessage(recipient, sender, ciphertext string) (string, error) {
	return "msg_123", nil
}

func (m *mockRelayClient) PostGroupChatMessage(recipients []string, sender, ciphertext string) ([]string, error) {
	return []string{"msg_grp_1"}, nil
}

func (m *mockRelayClient) GetPaste(id string) (string, error) {
	return "ciphertext_data", nil
}

func (m *mockRelayClient) ListenStream(handle string, onMessage func(msg client.StreamEvent), stopCh <-chan struct{}) error {
	return nil
}

func TestWebServer_HealthAndStatic(t *testing.T) {
	mockCl := &mockRelayClient{keys: make(map[string]*client.KeyInfo)}
	srv := NewServer(mockCl, "http://localhost:8080", "")
	handler := srv.Routes()

	// Test GET /
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for index.html, got %d", rec.Code)
	}

	// Test GET /api/health
	reqHealth := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected status 200 for health, got %d", recHealth.Code)
	}
}

func TestWebServer_IdentityAndSend(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "identity.json")

	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed generating identity: %v", err)
	}
	pub := id.Recipient().String()
	priv := id.String()
	fp := crypto.ComputeFingerprint(pub)

	err = storage.SaveIdentity(idPath, &storage.IdentityFile{
		Handle:      "PV-TESTER",
		PublicKey:   pub,
		PrivateKey:  priv,
		Fingerprint: fp,
	})
	if err != nil {
		t.Fatalf("failed saving identity: %v", err)
	}

	mockCl := &mockRelayClient{keys: make(map[string]*client.KeyInfo)}
	srv := NewServer(mockCl, "http://localhost:8080", idPath)
	handler := srv.Routes()

	// 1. Test /api/identity
	reqId := httptest.NewRequest(http.MethodGet, "/api/identity", nil)
	recId := httptest.NewRecorder()
	handler.ServeHTTP(recId, reqId)

	if recId.Code != http.StatusOK {
		t.Fatalf("expected 200 for identity, got %d: %s", recId.Code, recId.Body.String())
	}

	var idResp map[string]string
	if err := json.Unmarshal(recId.Body.Bytes(), &idResp); err != nil {
		t.Fatalf("failed unmarshaling identity: %v", err)
	}
	if idResp["handle"] != "PV-TESTER" {
		t.Fatalf("expected handle PV-TESTER, got %s", idResp["handle"])
	}

	// 2. Test /api/send
	sendPayload := SendRequest{
		Target: "PV-TARGET",
		Text:   "Secret web message",
	}
	body, _ := json.Marshal(sendPayload)
	reqSend := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	recSend := httptest.NewRecorder()
	handler.ServeHTTP(recSend, reqSend)

	if recSend.Code != http.StatusOK {
		t.Fatalf("expected 200 for send, got %d: %s", recSend.Code, recSend.Body.String())
	}

	// 3. Test /api/deposit
	depPayload := DepositRequest{
		Recipient: "PV-TARGET",
		Secret:    "Secret deposit payload",
		TTL:       300,
	}
	depBody, _ := json.Marshal(depPayload)
	reqDep := httptest.NewRequest(http.MethodPost, "/api/deposit", bytes.NewReader(depBody))
	recDep := httptest.NewRecorder()
	handler.ServeHTTP(recDep, reqDep)

	if recDep.Code != http.StatusOK {
		t.Fatalf("expected 200 for deposit, got %d: %s", recDep.Code, recDep.Body.String())
	}
}

func init() {
	_ = os.Setenv("PANDORA_TEST", "1")
}
