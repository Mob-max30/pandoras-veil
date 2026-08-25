package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MessageData represents a stored chat message.
type MessageData struct {
	ID         string `json:"id"`
	Sender     string `json:"sender"`
	Text       string `json:"text"`
	IsFile     bool   `json:"isFile"`
	FileName   string `json:"fileName,omitempty"`
	FileSize   string `json:"fileSize,omitempty"`
	FileType   string `json:"fileType,omitempty"`
	FileData   string `json:"fileData,omitempty"`
	Timestamp  string `json:"timestamp"`
	TTL        int    `json:"ttl"`
	ExpiresAt  int64  `json:"expiresAt"`
	IsOutgoing bool   `json:"isOutgoing"`
	IsGroup    bool   `json:"isGroup,omitempty"`
	GroupRoom  string `json:"groupRoom,omitempty"`
}

// ContactData represents a direct contact or group.
type ContactData struct {
	Handle      string   `json:"handle"`
	DisplayName string   `json:"displayName"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	PublicKey   string   `json:"publicKey,omitempty"`
	IsGroup     bool     `json:"isGroup"`
	Members     []string `json:"members,omitempty"`
	LastMessage string   `json:"lastMessage,omitempty"`
	LastTime    string   `json:"lastTime,omitempty"`
	Unread      int      `json:"unread,omitempty"`
}

// WebSessionData stores persistent conversation and contact state on the local server daemon.
type WebSessionData struct {
	Contacts      []ContactData            `json:"contacts"`
	Conversations map[string][]MessageData `json:"conversations"`
	MainTTLs      map[string]int           `json:"mainTTLs"`
}

var sessionMu sync.Mutex

func defaultSessionPath(configDir, filename string) (string, error) {
	if configDir != "" {
		return filepath.Join(configDir, filename), nil
	}
	baseDir, err := GetPandoraDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, filename), nil
}

// SaveWebSession saves the local server session data with 0600 file permissions.
func SaveWebSession(configDir string, data *WebSessionData) error {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	path, err := defaultSessionPath(configDir, "web_session.json")
	if err != nil {
		return fmt.Errorf("failed to get session path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create session dir: %w", err)
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	return os.WriteFile(path, bytes, 0600)
}

// LoadWebSession loads the local server session data.
func LoadWebSession(configDir string) (*WebSessionData, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	path, err := defaultSessionPath(configDir, "web_session.json")
	if err != nil {
		return nil, fmt.Errorf("failed to get session path: %w", err)
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &WebSessionData{
				Contacts:      []ContactData{},
				Conversations: make(map[string][]MessageData),
				MainTTLs:      make(map[string]int),
			}, nil
		}
		return nil, fmt.Errorf("failed to read session: %w", err)
	}

	var data WebSessionData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return &WebSessionData{
			Contacts:      []ContactData{},
			Conversations: make(map[string][]MessageData),
			MainTTLs:      make(map[string]int),
		}, nil
	}

	if data.Conversations == nil {
		data.Conversations = make(map[string][]MessageData)
	}
	if data.MainTTLs == nil {
		data.MainTTLs = make(map[string]int)
	}

	return &data, nil
}

// ClearWebSession completely removes the stored session data file.
func ClearWebSession(configDir string) error {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	path, err := defaultSessionPath(configDir, "web_session.json")
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
