package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultRelayURL = "https://pandoras-veil.onrender.com"

// Standard domain and HTTP errors
var (
	ErrNotFound          = errors.New("resource not found or already consumed/expired")
	ErrConflict          = errors.New("handle already registered")
	ErrRelayUnreachable  = errors.New("unable to reach relay backend")
	ErrKeyNotFound       = errors.New("recipient handle not found")
	ErrKeyConflict       = errors.New("handle already registered")
	ErrPasteNotFound     = errors.New("paste not found or already consumed")
	ErrInvalidRequest    = errors.New("invalid request parameters")
	ErrServerError       = errors.New("relay server error")
	ErrNetwork           = errors.New("network error connecting to relay server")
	ErrMalformedResponse = errors.New("malformed response received from relay server")
)

// RegisterKeyRequest represents the payload for POST /keys.
type RegisterKeyRequest struct {
	Handle    string `json:"handle,omitempty"`
	PublicKey string `json:"public_key"`
}

type KeyRegistrationRequest = RegisterKeyRequest

// RegisterKeyResponse represents the response for POST /keys.
type RegisterKeyResponse struct {
	Handle      string `json:"handle"`
	Fingerprint string `json:"fingerprint"`
}

// KeyResponse represents the response for GET /keys/:handle.
type KeyResponse struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// KeyInfo represents public key and fingerprint response
type KeyInfo struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// CreatePasteRequest represents the payload for POST /paste.
type CreatePasteRequest struct {
	Ciphertext       []byte `json:"ciphertext"`
	TTLSeconds       int    `json:"ttl_seconds"`
	BurnAfterReading bool   `json:"burn_after_reading"`
}

// PasteCreateRequest represents payload to store encrypted secret for CLI
type PasteCreateRequest struct {
	Ciphertext       string `json:"ciphertext"`
	Recipient        string `json:"recipient,omitempty"`
	Sender           string `json:"sender,omitempty"`
	TTLSeconds       int    `json:"ttl_seconds"`
	BurnAfterReading bool   `json:"burn_after_reading"`
}

// CreatePasteResponse represents the response for POST /paste.
type CreatePasteResponse struct {
	ID string `json:"id"`
}

type PasteCreateResponse = CreatePasteResponse

// GetPasteResponse represents the response for GET /paste/:id.
type GetPasteResponse struct {
	Ciphertext []byte `json:"ciphertext"`
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

// Option configures custom client settings.
type Option func(*Client)

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// Client provides HTTP access to the Pandora's Veil relay backend with context support.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a new HTTP client for the Pandora's Veil relay.
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) RegisterKey(ctx context.Context, handle, publicKey string) (*RegisterKeyResponse, error) {
	if strings.TrimSpace(publicKey) == "" {
		return nil, fmt.Errorf("%w: public key cannot be empty", ErrInvalidRequest)
	}

	reqBody := RegisterKeyRequest{
		Handle:    strings.TrimSpace(handle),
		PublicKey: strings.TrimSpace(publicKey),
	}

	var respObj RegisterKeyResponse
	err := c.doJSON(ctx, http.MethodPost, "/keys", reqBody, &respObj)
	if err != nil {
		return nil, err
	}

	return &respObj, nil
}

func (c *Client) GetKey(ctx context.Context, handle string) (*KeyResponse, error) {
	trimmedHandle := strings.TrimSpace(handle)
	if trimmedHandle == "" {
		return nil, fmt.Errorf("%w: handle cannot be empty", ErrInvalidRequest)
	}

	endpoint := fmt.Sprintf("/keys/%s", url.PathEscape(trimmedHandle))
	var respObj KeyResponse
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &respObj)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}

	return &respObj, nil
}

func (c *Client) CreatePaste(ctx context.Context, ciphertext []byte, ttlSeconds int, burnAfterReading bool) (*CreatePasteResponse, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("%w: ciphertext payload cannot be empty", ErrInvalidRequest)
	}

	reqBody := CreatePasteRequest{
		Ciphertext:       ciphertext,
		TTLSeconds:       ttlSeconds,
		BurnAfterReading: burnAfterReading,
	}

	var respObj CreatePasteResponse
	err := c.doJSON(ctx, http.MethodPost, "/paste", reqBody, &respObj)
	if err != nil {
		return nil, err
	}

	return &respObj, nil
}

func (c *Client) GetPaste(ctx context.Context, id string) (*GetPasteResponse, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, fmt.Errorf("%w: paste id cannot be empty", ErrInvalidRequest)
	}

	endpoint := fmt.Sprintf("/paste/%s", url.PathEscape(trimmedID))
	var respObj PasteResponse
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &respObj)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, ErrPasteNotFound
		}
		return nil, err
	}

	rawCiphertext := []byte(respObj.Ciphertext)
	if decoded, err := base64.StdEncoding.DecodeString(respObj.Ciphertext); err == nil && len(decoded) > 0 {
		rawCiphertext = decoded
	}

	return &GetPasteResponse{Ciphertext: rawCiphertext}, nil
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("%w: failed to build health request: %v", ErrInvalidRequest, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: server returned status %d", ErrServerError, resp.StatusCode)
	}

	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody interface{}, respTarget interface{}) error {
	fullURL := c.baseURL + path

	var bodyReader io.Reader
	if reqBody != nil {
		jsonBytes, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("%w: failed to marshal request body: %v", ErrInvalidRequest, err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("%w: failed to create HTTP request: %v", ErrInvalidRequest, err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapHTTPError(resp.StatusCode, path)
	}

	if respTarget != nil {
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("%w: failed to read response body: %v", ErrNetwork, err)
		}

		if err := json.Unmarshal(respBytes, respTarget); err != nil {
			return fmt.Errorf("%w: %v", ErrMalformedResponse, err)
		}
	}

	return nil
}

func mapHTTPError(statusCode int, path string) error {
	switch statusCode {
	case http.StatusNotFound:
		if strings.HasPrefix(path, "/paste/") {
			return ErrPasteNotFound
		}
		return ErrKeyNotFound
	case http.StatusConflict:
		return ErrKeyConflict
	case http.StatusBadRequest:
		return ErrInvalidRequest
	default:
		if statusCode >= 500 {
			return ErrServerError
		}
		return fmt.Errorf("%w: server returned status %d", ErrServerError, statusCode)
	}
}

// RelayClient defines the contract for CLI relay communication
type RelayClient interface {
	RegisterKey(handle, publicKey string) (*KeyInfo, error)
	GetKey(handle string) (*KeyInfo, error)
	DeleteKey(handle string) error
	PostPaste(ciphertext string, ttlSeconds int, burnAfterReading bool) (string, error)
	PostChatMessage(recipient, sender, ciphertext string) (string, error)
	PostGroupChatMessage(recipients []string, sender, ciphertext string) ([]string, error)
	GetPaste(id string) (string, error)
	FetchInbox(recipient, sender string) ([]InboxMessage, error)
	ListenStream(handle string, onMessage func(msg StreamEvent), stopCh <-chan struct{}) error
	Health() error
}

// HTTPClient implements RelayClient using standard net/http
type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewHTTPClient creates a new relay HTTP client for CLI
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

func (c *HTTPClient) GetKey(handle string) (*KeyInfo, error) {
	trimmedHandle := strings.TrimSpace(handle)
	if trimmedHandle == "" {
		return nil, fmt.Errorf("%w: handle cannot be empty", ErrInvalidRequest)
	}

	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/keys/%s", c.BaseURL, url.PathEscape(trimmedHandle)))
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

func (c *HTTPClient) DeleteKey(handle string) error {
	trimmedHandle := strings.TrimSpace(handle)
	if trimmedHandle == "" {
		return fmt.Errorf("%w: handle cannot be empty", ErrInvalidRequest)
	}

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/keys/%s", c.BaseURL, url.PathEscape(trimmedHandle)), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

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

func (c *HTTPClient) PostGroupChatMessage(recipients []string, sender, ciphertext string) ([]string, error) {
	var ids []string
	for _, rc := range recipients {
		trimmed := strings.TrimSpace(rc)
		if trimmed == "" {
			continue
		}
		id, err := c.PostChatMessage(trimmed, sender, ciphertext)
		if err != nil {
			return ids, fmt.Errorf("failed to send message to group member %s: %w", trimmed, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *HTTPClient) ListenStream(handle string, onMessage func(msg StreamEvent), stopCh <-chan struct{}) error {
	streamURL := fmt.Sprintf("%s/stream?handle=%s", c.BaseURL, handle)

	for {
		select {
		case <-stopCh:
			return nil
		default:
		}

		req, err := http.NewRequest("GET", streamURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create stream request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")

		streamClient := &http.Client{Timeout: 0}
		resp, err := streamClient.Do(req)
		if err != nil {
			select {
			case <-stopCh:
				return nil
			case <-time.After(2 * time.Second):
				continue
			}
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			select {
			case <-stopCh:
				return nil
			case <-time.After(2 * time.Second):
				continue
			}
		}

		done := make(chan struct{})
		go func() {
			select {
			case <-stopCh:
				resp.Body.Close()
			case <-done:
			}
		}()

		const maxCapacity = 16 * 1024 * 1024 // 16 MB max buffer for large file payloads
		buf := make([]byte, 64*1024)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(buf, maxCapacity)

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

		close(done)
		resp.Body.Close()

		select {
		case <-stopCh:
			return nil
		case <-time.After(1 * time.Second):
			// Auto-reconnect SSE stream
		}
	}
}

func (c *HTTPClient) GetPaste(id string) (string, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return "", fmt.Errorf("%w: paste id cannot be empty", ErrInvalidRequest)
	}

	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/paste/%s", c.BaseURL, url.PathEscape(trimmedID)))
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

type InboxMessage struct {
	ID         string `json:"id"`
	Ciphertext string `json:"ciphertext"`
	Sender     string `json:"sender"`
}

type FetchInboxResponse struct {
	Messages []InboxMessage `json:"messages"`
}

func (c *HTTPClient) FetchInbox(recipient, sender string) ([]InboxMessage, error) {
	endpoint := fmt.Sprintf("%s/inbox?recipient=%s", c.BaseURL, url.QueryEscape(recipient))
	if sender != "" {
		endpoint += fmt.Sprintf("&sender=%s", url.QueryEscape(sender))
	}

	resp, err := c.HTTPClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("relay returned status %d: %s", resp.StatusCode, string(body))
	}

	var inboxResp FetchInboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&inboxResp); err != nil {
		return nil, fmt.Errorf("failed to decode inbox response: %w", err)
	}
	return inboxResp.Messages, nil
}

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

// MockClient is an in-memory mock implementation of RelayClient for standalone CLI testing
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

func (m *MockClient) DeleteKey(handle string) error {
	delete(m.Keys, handle)
	return nil
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

func (m *MockClient) PostGroupChatMessage(recipients []string, sender, ciphertext string) ([]string, error) {
	var ids []string
	for _, rc := range recipients {
		id, err := m.PostChatMessage(rc, sender, ciphertext)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
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

func (m *MockClient) FetchInbox(recipient, sender string) ([]InboxMessage, error) {
	return []InboxMessage{}, nil
}
