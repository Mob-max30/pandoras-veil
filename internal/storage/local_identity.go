package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// IdentityFile is the data stored locally on disk
type IdentityFile struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key"`
	PrivateKey  string `json:"private_key"`
	Fingerprint string `json:"fingerprint"`
}

// DefaultIdentityPath returns the default path for ~/.pandora/identity.json
func DefaultIdentityPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home dir: %w", err)
	}
	return filepath.Join(home, ".pandora", "identity.json"), nil
}

// SaveIdentity writes the identity to disk with restricted 0600 permissions
func SaveIdentity(path string, id *IdentityFile) error {
	if path == "" {
		var err error
		path, err = DefaultIdentityPath()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode identity file: %w", err)
	}

	// Write file with strict 0600 permissions (read/write only by owner)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	// Double check permissions (especially on Unix systems)
	_ = os.Chmod(path, 0600)
	return nil
}

// LoadIdentity reads the local identity from disk
func LoadIdentity(path string) (*IdentityFile, error) {
	if path == "" {
		var err error
		path, err = DefaultIdentityPath()
		if err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("no device identity found. Run 'pandora init' first")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var id IdentityFile
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("failed to parse identity file: %w", err)
	}

	return &id, nil
}
