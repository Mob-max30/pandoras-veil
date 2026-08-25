package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

func TestSaveAndLoadIdentity(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "identity")

	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("crypto.GenerateIdentity failed: %v", err)
	}

	originalSecretKey := id.String()

	err = storage.SaveIdentityToFile(originalSecretKey, filePath)
	if err != nil {
		t.Fatalf("SaveIdentityToFile failed: %v", err)
	}

	loadedSecretKey, err := storage.LoadIdentityFromFile(filePath)
	if err != nil {
		t.Fatalf("LoadIdentityFromFile failed: %v", err)
	}

	if loadedSecretKey != originalSecretKey {
		t.Errorf("loaded secret key mismatch: expected %s, got %s", originalSecretKey, loadedSecretKey)
	}

	// Verify loaded identity parses back correctly to same public key
	loadedId, err := crypto.ParseIdentity(loadedSecretKey)
	if err != nil {
		t.Fatalf("crypto.ParseIdentity failed for loaded secret key: %v", err)
	}

	if crypto.GetPublicKey(loadedId) != crypto.GetPublicKey(id) {
		t.Errorf("public key mismatch between original and loaded identity")
	}
}

func TestIdentityFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping Unix permission check on Windows OS")
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "identity")

	err := storage.SaveIdentityToFile("AGE-SECRET-KEY-TEST", filePath)
	if err != nil {
		t.Fatalf("SaveIdentityToFile failed: %v", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("os.Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}
}

func TestLoadMissingIdentity(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "non_existent_identity")

	_, err := storage.LoadIdentityFromFile(filePath)
	if err == nil {
		t.Fatal("expected error loading missing identity file, got nil")
	}

	if !errors.Is(err, storage.ErrIdentityNotFound) {
		t.Errorf("expected ErrIdentityNotFound error, got %v", err)
	}
}

func TestSaveEmptyIdentity(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "identity")

	err := storage.SaveIdentityToFile("   ", filePath)
	if err == nil {
		t.Fatal("expected error saving empty identity, got nil")
	}

	if !errors.Is(err, storage.ErrEmptyIdentity) {
		t.Errorf("expected ErrEmptyIdentity error, got %v", err)
	}
}

func TestGetPandoraDirAndPath(t *testing.T) {
	dir, err := storage.GetPandoraDir()
	if err != nil {
		t.Fatalf("GetPandoraDir failed: %v", err)
	}
	if filepath.Base(dir) != ".pandora" {
		t.Errorf("expected directory base name '.pandora', got %s", filepath.Base(dir))
	}

	path, err := storage.GetIdentityPath()
	if err != nil {
		t.Fatalf("GetIdentityPath failed: %v", err)
	}
	if filepath.Base(path) != "identity" {
		t.Errorf("expected path base name 'identity', got %s", filepath.Base(path))
	}
}
