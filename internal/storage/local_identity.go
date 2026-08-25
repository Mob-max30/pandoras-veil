package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
)

var (
	// ErrIdentityNotFound is returned when attempting to load a non-existent identity file.
	ErrIdentityNotFound = errors.New("identity file not found")

	// ErrEmptyIdentity is returned when an identity string is empty.
	ErrEmptyIdentity = errors.New("identity content is empty")
)

// IdentityFile is the structured credential stored locally on disk
type IdentityFile struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key"`
	PrivateKey  string `json:"private_key"`
	Fingerprint string `json:"fingerprint"`
}

// GetPandoraDir returns the default directory path for Pandora data (~/.pandora).
func GetPandoraDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".pandora"), nil
}

// GetIdentityPath returns the default file path for local identity storage (~/.pandora/identity).
func GetIdentityPath() (string, error) {
	dir, err := GetPandoraDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity"), nil
}

// DefaultIdentityPath returns the default path for ~/.pandora/identity.json
func DefaultIdentityPath() (string, error) {
	dir, err := GetPandoraDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.json"), nil
}

// EnsurePandoraDir creates the specified directory path with 0700 permissions if it does not exist.
func EnsurePandoraDir(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}
	return nil
}

// SaveIdentityToFile writes an age identity string to the specified file path with 0600 permissions.
func SaveIdentityToFile(identityStr string, filePath string) error {
	trimmed := strings.TrimSpace(identityStr)
	if trimmed == "" {
		return ErrEmptyIdentity
	}

	dir := filepath.Dir(filePath)
	if err := EnsurePandoraDir(dir); err != nil {
		return err
	}

	// Write file with restrictive 0600 permissions.
	content := []byte(trimmed + "\n")
	if err := os.WriteFile(filePath, content, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	_ = os.Chmod(filePath, 0600)
	return nil
}

// LoadIdentityFromFile reads and returns the age identity string from the specified file path.
func LoadIdentityFromFile(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrIdentityNotFound
		}
		return "", fmt.Errorf("failed to read identity file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", ErrEmptyIdentity
	}

	return content, nil
}

// SaveDefaultIdentity saves the identity to the default location (~/.pandora/identity).
func SaveDefaultIdentity(identityStr string) error {
	path, err := GetIdentityPath()
	if err != nil {
		return err
	}
	return SaveIdentityToFile(identityStr, path)
}

// LoadDefaultIdentity loads the identity from the default location (~/.pandora/identity).
func LoadDefaultIdentity() (string, error) {
	path, err := GetIdentityPath()
	if err != nil {
		return "", err
	}
	return LoadIdentityFromFile(path)
}

// SaveIdentity writes the structured identity to disk with restricted 0600 permissions
func SaveIdentity(path string, id *IdentityFile) error {
	if path == "" {
		var err error
		path, err = DefaultIdentityPath()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := EnsurePandoraDir(dir); err != nil {
		return err
	}

	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode identity file: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	_ = os.Chmod(path, 0600)
	return nil
}

// LoadIdentity reads the local identity from disk (handles JSON format and raw key format)
func LoadIdentity(path string) (*IdentityFile, error) {
	if path == "" {
		var err error
		path, err = DefaultIdentityPath()
		if err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, ErrIdentityNotFound
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var id IdentityFile
	if err := json.Unmarshal(data, &id); err == nil && id.PrivateKey != "" {
		return &id, nil
	}

	// Try reading as raw secret key
	secretKey := strings.TrimSpace(string(data))
	if secretKey != "" {
		parsedId, err := age.ParseX25519Identity(secretKey)
		if err == nil {
			pubKey := parsedId.Recipient().String()
			fp := crypto.Fingerprint(pubKey)
			return &IdentityFile{
				Handle:      "PV-LOCAL",
				PublicKey:   pubKey,
				PrivateKey:  secretKey,
				Fingerprint: fp,
			}, nil
		}
	}

	return nil, errors.New("failed to parse device identity")
}
