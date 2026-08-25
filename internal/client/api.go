package client

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultRelayURL = "https://pandoras-veil.onrender.com"

// Common errors returned by the relay client
var (
	ErrNotFound         = errors.New("resource not found or already consumed/expired")
	ErrConflict         = errors.New("handle already registered")
	ErrRelayUnreachable = errors.New("unable to reach relay backend")
)

// KeyRegistrationRequest represents the payload for registering a public key
type KeyRegistrationRequest struct {
	Handle    string `json:"handle,omitempty"`
	PublicKey string `json:"public_key"`
}

// KeyInfo represents public key and fingerprint response
type KeyInfo struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// PasteCreateRequest represents payload to store encrypted secret
type PasteCreateRequest struct {
	Ciphertext       string `json:"ciphertext"`
	Recipient        string `json:"recipient,omitempty"`
	Sender           string `json:"sender,omitempty"`
	TTLSeconds       int    `json:"ttl_seconds"`
	BurnAfterReading bool   `json:"burn_after_reading"`
}

// PasteCreateResponse represents response after creating a paste
type PasteCreateResponse struct {
	ID string `json:"id"`
}

// PasteResponse represents retrieved ciphertext
type PasteResponse struct {
	Ciphertext string `json:"ciphertext"`
}

// StreamEvent represents incoming live message event
type StreamEvent struct {
	ID         string `json:"id"`
	Ciphertext string `json:"ciphertext"`
	Sender     string `json:"sender,omitempty"`
}

// Client defines the contract for relay communication
type Client interface {
	RegisterKey(handle, publicKey string) (*KeyInfo, error)
	GetKey(handle string) (*KeyInfo, error)
	PostPaste(ciphertext string, ttlSeconds int, burnAfterReading bool) (string, error)
	PostChatMessage(recipient, sender, ciphertext string) (string, error)
	GetPaste(id string) (string, error)
	ListenStream(handle string, onMessage func(msg StreamEvent), stopCh <-chan struct{}) error
	Health() error
}

// HTTPClient implements Client using standard net/http
type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewHTTPClient creates a new relay HTTP client
func NewHTTPClient(baseURL string) *HTTPClient {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = DefaultRelayURL
	}
	return &HTTPClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// RegisterKey calls POST /keys
func (c *HTTPClient) RegisterKey(handle, publicKey string) (*KeyInfo, error) {
	reqBody := KeyRegistrationRequest{
		Handle:    handle,
		PublicKey: publicKey,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/keys", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, ErrConflict
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}

	var keyInfo KeyInfo
	if err := json.NewDecoder(resp.Body).Decode(&keyInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &keyInfo, nil
}

// GetKey calls GET /keys/:handle
func (c *HTTPClient) GetKey(handle string) (*KeyInfo, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/keys/%s", c.BaseURL, handle))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}

	var keyInfo KeyInfo
	if err := json.NewDecoder(resp.Body).Decode(&keyInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &keyInfo, nil
}

// PostPaste calls POST /paste
func (c *HTTPClient) PostPaste(ciphertext string, ttlSeconds int, burnAfterReading bool) (string, error) {
	b64Ciphertext := ciphertext
	if _, err := base64.StdEncoding.DecodeString(ciphertext); err != nil {
		b64Ciphertext = base64.StdEncoding.EncodeToString([]byte(ciphertext))
	}

	reqBody := PasteCreateRequest{
		Ciphertext:       b64Ciphertext,
		TTLSeconds:       ttlSeconds,
		BurnAfterReading: burnAfterReading,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/paste", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}

	var createResp PasteCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return createResp.ID, nil
}

// PostChatMessage calls POST /paste with recipient and sender routing metadata
func (c *HTTPClient) PostChatMessage(recipient, sender, ciphertext string) (string, error) {
	b64Ciphertext := ciphertext
	if _, err := base64.StdEncoding.DecodeString(ciphertext); err != nil {
		b64Ciphertext = base64.StdEncoding.EncodeToString([]byte(ciphertext))
	}

	reqBody := PasteCreateRequest{
		Ciphertext:       b64Ciphertext,
		Recipient:        recipient,
		Sender:           sender,
		TTLSeconds:       86400,
		BurnAfterReading: false,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/paste", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}

	var createResp PasteCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return createResp.ID, nil
}

// ListenStream opens an SSE connection to GET /stream?handle=<handle> and streams incoming events
func (c *HTTPClient) ListenStream(handle string, onMessage func(msg StreamEvent), stopCh <-chan struct{}) error {
	streamURL := fmt.Sprintf("%s/stream?handle=%s", c.BaseURL, handle)
	req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Use custom client without short timeout for long-lived SSE stream
	streamClient := &http.Client{Timeout: 0}
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream connection failed with status %d", resp.StatusCode)
	}

	go func() {
		<-stopCh
		resp.Body.Close()
	}()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payload := strings.TrimPrefix(line, "data: ")
			var event StreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err == nil {
				if decoded, err := base64.StdEncoding.DecodeString(event.Ciphertext); err == nil {
					event.Ciphertext = string(decoded)
				}
				onMessage(event)
			}
		}
	}
	return nil
}

// GetPaste calls GET /paste/:id
func (c *HTTPClient) GetPaste(id string) (string, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/paste/%s", c.BaseURL, id))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}

	var pasteResp PasteResponse
	if err := json.NewDecoder(resp.Body).Decode(&pasteResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(pasteResp.Ciphertext); err == nil {
		return string(decoded), nil
	}
	return pasteResp.Ciphertext, nil
}

// Health calls GET /health
func (c *HTTPClient) Health() error {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/health")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay health check failed with status %d", resp.StatusCode)
	}
	return nil
}

// MockClient is an in-memory mock implementation of Client for standalone CLI testing
type MockClient struct {
	Keys   map[string]*KeyInfo
	Pastes map[string]PasteCreateRequest
}

// NewMockClient creates a new in-memory MockClient
func NewMockClient() *MockClient {
	return &MockClient{
		Keys:   make(map[string]*KeyInfo),
		Pastes: make(map[string]PasteCreateRequest),
	}
}

func (m *MockClient) RegisterKey(handle, publicKey string) (*KeyInfo, error) {
	if handle == "" {
		handle = fmt.Sprintf("PV-MOCK-%04d", len(m.Keys)+1)
	}
	if _, exists := m.Keys[handle]; exists {
		return nil, ErrConflict
	}
	info := &KeyInfo{
		Handle:      handle,
		PublicKey:   publicKey,
		Fingerprint: "7C91-42AE",
	}
	m.Keys[handle] = info
	return info, nil
}

func (m *MockClient) GetKey(handle string) (*KeyInfo, error) {
	info, exists := m.Keys[handle]
	if !exists {
		return nil, ErrNotFound
	}
	return info, nil
}

func (m *MockClient) PostPaste(ciphertext string, ttlSeconds int, burnAfterReading bool) (string, error) {
	id := fmt.Sprintf("pv_%d", time.Now().UnixNano())
	m.Pastes[id] = PasteCreateRequest{
		Ciphertext:       ciphertext,
		TTLSeconds:       ttlSeconds,
		BurnAfterReading: burnAfterReading,
	}
	return id, nil
}

func (m *MockClient) PostChatMessage(recipient, sender, ciphertext string) (string, error) {
	id := fmt.Sprintf("pv_%d", time.Now().UnixNano())
	m.Pastes[id] = PasteCreateRequest{
		Ciphertext:       ciphertext,
		Recipient:        recipient,
		Sender:           sender,
		TTLSeconds:       86400,
		BurnAfterReading: false,
	}
	return id, nil
}

func (m *MockClient) GetPaste(id string) (string, error) {
	paste, exists := m.Pastes[id]
	if !exists {
		return "", ErrNotFound
	}
	if paste.BurnAfterReading {
		delete(m.Pastes, id)
	}
	return paste.Ciphertext, nil
}

func (m *MockClient) ListenStream(handle string, onMessage func(msg StreamEvent), stopCh <-chan struct{}) error {
	<-stopCh
	return nil
}

func (m *MockClient) Health() error {
	return nil
}
