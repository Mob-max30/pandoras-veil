package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
)

// 1. Test Register Key Success
func TestRegisterKeySuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/keys" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "bad route", http.StatusBadRequest)
			return
		}

		var req client.RegisterKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		if req.PublicKey != "age1testkey" {
			t.Errorf("expected public_key 'age1testkey', got '%s'", req.PublicKey)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(client.RegisterKeyResponse{
			Handle:      "PV-A8F4-92KD",
			Fingerprint: "7C91-42AE",
		})
	}))
	defer ts.Close()

	c := client.NewClient(ts.URL)
	resp, err := c.RegisterKey(context.Background(), "PV-A8F4-92KD", "age1testkey")
	if err != nil {
		t.Fatalf("RegisterKey failed: %v", err)
	}

	if resp.Handle != "PV-A8F4-92KD" {
		t.Errorf("expected handle 'PV-A8F4-92KD', got '%s'", resp.Handle)
	}
	if resp.Fingerprint != "7C91-42AE" {
		t.Errorf("expected fingerprint '7C91-42AE', got '%s'", resp.Fingerprint)
	}
}

// 2. Test Retrieve Recipient Key
func TestGetKeySuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/keys/PV-A8F4-92KD" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(client.KeyResponse{
			PublicKey:   "age1recipientkey",
			Fingerprint: "7C91-42AE",
		})
	}))
	defer ts.Close()

	c := client.NewClient(ts.URL)
	resp, err := c.GetKey(context.Background(), "PV-A8F4-92KD")
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}

	if resp.PublicKey != "age1recipientkey" {
		t.Errorf("expected public_key 'age1recipientkey', got '%s'", resp.PublicKey)
	}
	if resp.Fingerprint != "7C91-42AE" {
		t.Errorf("expected fingerprint '7C91-42AE', got '%s'", resp.Fingerprint)
	}
}

// 3. Test Upload Ciphertext
func TestCreatePasteSuccess(t *testing.T) {
	expectedCiphertext := []byte("age-encrypted-ciphertext-bytes")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/paste" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "bad route", http.StatusBadRequest)
			return
		}

		var req client.CreatePasteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		if string(req.Ciphertext) != string(expectedCiphertext) {
			t.Errorf("ciphertext mismatch")
		}
		if req.TTLSeconds != 3600 {
			t.Errorf("expected ttl_seconds 3600, got %d", req.TTLSeconds)
		}
		if !req.BurnAfterReading {
			t.Errorf("expected burn_after_reading true")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(client.CreatePasteResponse{
			ID: "paste-id-12345",
		})
	}))
	defer ts.Close()

	c := client.NewClient(ts.URL)
	resp, err := c.CreatePaste(context.Background(), expectedCiphertext, 3600, true)
	if err != nil {
		t.Fatalf("CreatePaste failed: %v", err)
	}

	if resp.ID != "paste-id-12345" {
		t.Errorf("expected paste ID 'paste-id-12345', got '%s'", resp.ID)
	}
}

// 4. Test Retrieve Ciphertext
func TestGetPasteSuccess(t *testing.T) {
	expectedCiphertext := []byte("retrieved-age-ciphertext-payload")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/paste/paste-id-12345" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(client.GetPasteResponse{
			Ciphertext: expectedCiphertext,
		})
	}))
	defer ts.Close()

	c := client.NewClient(ts.URL)
	resp, err := c.GetPaste(context.Background(), "paste-id-12345")
	if err != nil {
		t.Fatalf("GetPaste failed: %v", err)
	}

	if string(resp.Ciphertext) != string(expectedCiphertext) {
		t.Errorf("ciphertext payload mismatch")
	}
}

// 5. Test Health Check
func TestHealthSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not health", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := client.NewClient(ts.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

// 6. Test Non-2xx Response Handling (404, 409, 400, 500)
func TestNon2xxResponseHandling(t *testing.T) {
	testCases := []struct {
		name          string
		statusCode    int
		callFunc      func(c *client.Client) error
		expectedError error
	}{
		{
			name:       "404 Key Not Found",
			statusCode: http.StatusNotFound,
			callFunc: func(c *client.Client) error {
				_, err := c.GetKey(context.Background(), "unknown-handle")
				return err
			},
			expectedError: client.ErrKeyNotFound,
		},
		{
			name:       "404 Paste Not Found",
			statusCode: http.StatusNotFound,
			callFunc: func(c *client.Client) error {
				_, err := c.GetPaste(context.Background(), "unknown-paste-id")
				return err
			},
			expectedError: client.ErrPasteNotFound,
		},
		{
			name:       "409 Handle Conflict",
			statusCode: http.StatusConflict,
			callFunc: func(c *client.Client) error {
				_, err := c.RegisterKey(context.Background(), "existing-handle", "age1key")
				return err
			},
			expectedError: client.ErrKeyConflict,
		},
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			callFunc: func(c *client.Client) error {
				_, err := c.CreatePaste(context.Background(), []byte("data"), 0, false)
				return err
			},
			expectedError: client.ErrInvalidRequest,
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			callFunc: func(c *client.Client) error {
				return c.Health(context.Background())
			},
			expectedError: client.ErrServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer ts.Close()

			c := client.NewClient(ts.URL)
			err := tc.callFunc(c)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !errors.Is(err, tc.expectedError) {
				t.Errorf("expected error %v, got %v", tc.expectedError, err)
			}
		})
	}
}

// 7. Test Malformed Response Handling
func TestMalformedResponseHandling(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{ invalid-json-payload "))
	}))
	defer ts.Close()

	c := client.NewClient(ts.URL)
	_, err := c.GetKey(context.Background(), "handle")
	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
	if !errors.Is(err, client.ErrMalformedResponse) {
		t.Errorf("expected ErrMalformedResponse, got %v", err)
	}
}

// 8. Test Network Failure
func TestNetworkFailure(t *testing.T) {
	// Point client to non-existent server / closed port
	c := client.NewClient("http://127.0.0.1:59999", client.WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}))
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !errors.Is(err, client.ErrNetwork) {
		t.Errorf("expected ErrNetwork, got %v", err)
	}
}
