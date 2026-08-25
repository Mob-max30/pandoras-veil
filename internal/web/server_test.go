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

func (m *mockRelayClient) PostChatMessageWithOptions(recipient, sender, ciphertext string, ttlSeconds int, burnAfterReading bool) (string, error) {
	return "msg_123", nil
}

func (m *mockRelayClient) PostGroupChatMessage(recipients []string, sender, ciphertext string) ([]string, error) {
	return []string{"msg_grp_1"}, nil
}

func (m *mockRelayClient) PostGroupChatMessageWithOptions(recipients []string, sender, ciphertext string, ttlSeconds int, burnAfterReading bool) ([]string, error) {
	return []string{"msg_grp_1"}, nil
}

func (m *mockRelayClient) GetPaste(id string) (string, error) {
	return "ciphertext_data", nil
}

func (m *mockRelayClient) FetchInbox(recipient, sender string) ([]client.InboxMessage, error) {
	return nil, nil
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

	// Verify session token is injected in HTML
	if !bytes.Contains(rec.Body.Bytes(), []byte(`name="pandora-token"`)) {
		t.Fatalf("expected pandora-token meta tag in index.html response")
	}

	// Test GET /api/health
	reqHealth := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected status 200 for health, got %d", recHealth.Code)
	}
}

func TestWebServer_CSRFAndDNSRebindingDefense(t *testing.T) {
	mockCl := &mockRelayClient{keys: make(map[string]*client.KeyInfo)}
	srv := NewServer(mockCl, "http://localhost:8080", "")
	handler := srv.Routes()

	// 1. DNS Rebinding Attack: Malicious Host Header
	reqRebind := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	reqRebind.Host = "evil-attacker.com"
	recRebind := httptest.NewRecorder()
	handler.ServeHTTP(recRebind, reqRebind)

	if recRebind.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on foreign host header, got %d", recRebind.Code)
	}

	// 2. Cross-Origin Localhost CSRF Attack: Foreign Origin Header
	reqCSRF := httptest.NewRequest(http.MethodGet, "/api/identity", nil)
	reqCSRF.Header.Set("Origin", "https://malicious-website.com")
	reqCSRF.Header.Set("X-Pandora-Token", srv.SessionToken())
	recCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recCSRF, reqCSRF)

	if recCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on foreign origin header, got %d", recCSRF.Code)
	}

	// 3. Unauthorized API Access without Token
	reqNoToken := httptest.NewRequest(http.MethodGet, "/api/identity", nil)
	recNoToken := httptest.NewRecorder()
	handler.ServeHTTP(recNoToken, reqNoToken)

	if recNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without session token, got %d", recNoToken.Code)
	}
}

func TestWebServer_IdentityAndSendWithToken(t *testing.T) {
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

	// 1. Test /api/identity with token
	reqId := httptest.NewRequest(http.MethodGet, "/api/identity", nil)
	reqId.Header.Set("X-Pandora-Token", srv.SessionToken())
	recId := httptest.NewRecorder()
	handler.ServeHTTP(recId, reqId)

	if recId.Code != http.StatusOK {
		t.Fatalf("expected 200 for identity, got %d: %s", recId.Code, recId.Body.String())
	}

	var idResp map[string]any
	if err := json.Unmarshal(recId.Body.Bytes(), &idResp); err != nil {
		t.Fatalf("failed unmarshaling identity: %v", err)
	}
	if idResp["handle"] != "PV-TESTER" {
		t.Fatalf("expected handle PV-TESTER, got %v", idResp["handle"])
	}

	// 2. Test /api/send with token
	sendPayload := SendRequest{
		Target: "PV-TARGET",
		Text:   "Secret web message",
	}
	body, _ := json.Marshal(sendPayload)
	reqSend := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	reqSend.Header.Set("X-Pandora-Token", srv.SessionToken())
	recSend := httptest.NewRecorder()
	handler.ServeHTTP(recSend, reqSend)

	if recSend.Code != http.StatusOK {
		t.Fatalf("expected 200 for send, got %d: %s", recSend.Code, recSend.Body.String())
	}

	// 3. Test /api/deposit with token
	depPayload := DepositRequest{
		Recipient: "PV-TARGET",
		Secret:    "Secret deposit payload",
		TTL:       300,
	}
	depBody, _ := json.Marshal(depPayload)
	reqDep := httptest.NewRequest(http.MethodPost, "/api/deposit", bytes.NewReader(depBody))
	reqDep.Header.Set("X-Pandora-Token", srv.SessionToken())
	recDep := httptest.NewRecorder()
	handler.ServeHTTP(recDep, reqDep)

	if recDep.Code != http.StatusOK {
		t.Fatalf("expected 200 for deposit, got %d: %s", recDep.Code, recDep.Body.String())
	}
}

func init() {
	_ = os.Setenv("PANDORA_TEST", "1")
}
