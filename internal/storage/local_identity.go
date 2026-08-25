package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrIdentityNotFound is returned when attempting to load a non-existent identity file.
	ErrIdentityNotFound = errors.New("identity file not found")

	// ErrEmptyIdentity is returned when an identity string is empty.
	ErrEmptyIdentity = errors.New("identity content is empty")
)

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

	// Enforce 0600 permissions explicitly in case the file already existed with different permissions.
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("failed to set restrictive permissions 0600 on identity file: %w", err)
	}

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
