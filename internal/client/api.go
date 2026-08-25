package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrKeyNotFound is returned when a recipient handle is not registered.
	ErrKeyNotFound = errors.New("recipient handle not found")

	// ErrKeyConflict is returned when attempting to register an already existing handle.
	ErrKeyConflict = errors.New("handle already registered")

	// ErrPasteNotFound is returned when a requested paste is expired, consumed, or non-existent.
	ErrPasteNotFound = errors.New("paste not found or already consumed")

	// ErrInvalidRequest is returned when the request payload or parameters are invalid.
	ErrInvalidRequest = errors.New("invalid request parameters")

	// ErrServerError is returned when the relay backend encounters an internal error.
	ErrServerError = errors.New("relay server error")

	// ErrNetwork is returned when a network or connection error occurs while reaching the relay.
	ErrNetwork = errors.New("network error connecting to relay server")

	// ErrMalformedResponse is returned when the relay returns invalid or unexpected JSON.
	ErrMalformedResponse = errors.New("malformed response received from relay server")
)

// RegisterKeyRequest represents the payload for POST /keys.
type RegisterKeyRequest struct {
	Handle    string `json:"handle,omitempty"`
	PublicKey string `json:"public_key"`
}

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

// CreatePasteRequest represents the payload for POST /paste.
type CreatePasteRequest struct {
	Ciphertext       []byte `json:"ciphertext"`
	TTLSeconds       int    `json:"ttl_seconds"`
	BurnAfterReading bool   `json:"burn_after_reading"`
}

// CreatePasteResponse represents the response for POST /paste.
type CreatePasteResponse struct {
	ID string `json:"id"`
}

// GetPasteResponse represents the response for GET /paste/:id.
type GetPasteResponse struct {
	Ciphertext []byte `json:"ciphertext"`
}

// Client provides HTTP access to the Pandora's Veil relay backend.
type Client struct {
	baseURL    string
	httpClient *http.Client
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

// RegisterKey registers a public key (and optional handle) with the relay server.
// Endpoint: POST /keys
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

// GetKey fetches the recipient public key and fingerprint for a registered handle.
// Endpoint: GET /keys/:handle
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

// CreatePaste uploads an encrypted ciphertext payload to the relay server.
// Endpoint: POST /paste
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

// GetPaste retrieves encrypted ciphertext payload by paste ID from the relay server.
// Endpoint: GET /paste/:id
func (c *Client) GetPaste(ctx context.Context, id string) (*GetPasteResponse, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, fmt.Errorf("%w: paste id cannot be empty", ErrInvalidRequest)
	}

	endpoint := fmt.Sprintf("/paste/%s", url.PathEscape(trimmedID))
	var respObj GetPasteResponse
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &respObj)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, ErrPasteNotFound
		}
		return nil, err
	}

	if len(respObj.Ciphertext) == 0 {
		return nil, ErrPasteNotFound
	}

	return &respObj, nil
}

// Health checks the liveness of the relay backend.
// Endpoint: GET /health
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

// doJSON is a helper for performing HTTP requests with JSON request/response payloads.
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

// mapHTTPError converts HTTP status codes to domain errors without revealing sensitive details.
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
