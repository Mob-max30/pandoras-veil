package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

func TestCLI_InitAndIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "identity.json")

	mockClient := client.NewMockClient()

	// 1. Run 'init'
	var outBuf bytes.Buffer
	ui := NewUI(strings.NewReader(""), &outBuf)

	exitCode := runInit([]string{"--config", configPath, "--handle", "PV-ALICE"}, ui, mockClient)
	if exitCode != 0 {
		t.Fatalf("runInit failed with code %d. Output: %s", exitCode, outBuf.String())
	}

	if !strings.Contains(outBuf.String(), "Device initialized successfully") {
		t.Errorf("expected success message in output, got: %s", outBuf.String())
	}

	// Verify identity was written to disk
	idFile, err := storage.LoadIdentity(configPath)
	if err != nil {
		t.Fatalf("failed to load saved identity: %v", err)
	}
	if idFile.Handle != "PV-ALICE" {
		t.Errorf("expected handle PV-ALICE, got %s", idFile.Handle)
	}
	if idFile.Fingerprint == "" {
		t.Errorf("expected non-empty fingerprint")
	}

	// 2. Run 'identity'
	outBuf.Reset()
	ui = NewUI(strings.NewReader(""), &outBuf)
	exitCode = runIdentity([]string{"--config", configPath}, ui, mockClient)
	if exitCode != 0 {
		t.Fatalf("runIdentity failed with code %d", exitCode)
	}

	if !strings.Contains(outBuf.String(), "PV-ALICE") || !strings.Contains(outBuf.String(), idFile.Fingerprint) {
		t.Errorf("expected identity output to contain handle and fingerprint, got: %s", outBuf.String())
	}
}

func TestCLI_SendVerificationHardStop_Rejection(t *testing.T) {
	mockClient := client.NewMockClient()
	_, _ = mockClient.RegisterKey("PV-BOB", "age1mockbobkey...")

	var outBuf bytes.Buffer
	// User responds 'n' to confirmation prompt
	inputReader := strings.NewReader("n\n")
	ui := NewUI(inputReader, &outBuf)

	exitCode := runSend([]string{"--to", "PV-BOB", "SuperSecretData"}, ui, mockClient)
	if exitCode != 1 {
		t.Errorf("expected exit code 1 when user rejects fingerprint confirmation, got %d", exitCode)
	}

	if !strings.Contains(outBuf.String(), "verification aborted") {
		t.Errorf("expected verification aborted message, got: %s", outBuf.String())
	}

	// Verify no paste was created
	if len(mockClient.Pastes) != 0 {
		t.Errorf("no paste should be created on aborted verification, found %d", len(mockClient.Pastes))
	}
}

func TestCLI_EndToEndSendAndReadWithDecryption(t *testing.T) {
	tmpDir := t.TempDir()
	aliceConfig := filepath.Join(tmpDir, "alice.json")
	bobConfig := filepath.Join(tmpDir, "bob.json")
	eveConfig := filepath.Join(tmpDir, "eve.json")

	mockClient := client.NewMockClient()

	// 1. Initialize Bob (Recipient)
	var outBuf bytes.Buffer
	ui := NewUI(strings.NewReader(""), &outBuf)
	if code := runInit([]string{"--config", bobConfig, "--handle", "PV-BOB"}, ui, mockClient); code != 0 {
		t.Fatalf("failed to init Bob: %s", outBuf.String())
	}

	// 2. Initialize Eve (Unauthorized Attacker)
	outBuf.Reset()
	if code := runInit([]string{"--config", eveConfig, "--handle", "PV-EVE"}, ui, mockClient); code != 0 {
		t.Fatalf("failed to init Eve: %s", outBuf.String())
	}

	// 3. Initialize Alice (Sender)
	outBuf.Reset()
	if code := runInit([]string{"--config", aliceConfig, "--handle", "PV-ALICE"}, ui, mockClient); code != 0 {
		t.Fatalf("failed to init Alice: %s", outBuf.String())
	}

	// 4. Alice sends secret to Bob, confirms prompt with 'y'
	outBuf.Reset()
	sendInput := strings.NewReader("y\n")
	ui = NewUI(sendInput, &outBuf)

	secretText := "CONFIDENTIAL_PAYLOAD_98765"
	exitCode := runSend([]string{"--to", "PV-BOB", secretText}, ui, mockClient)
	if exitCode != 0 {
		t.Fatalf("send failed with code %d: %s", exitCode, outBuf.String())
	}

	// Find created paste ID
	if len(mockClient.Pastes) != 1 {
		t.Fatalf("expected 1 paste in mock relay, found %d", len(mockClient.Pastes))
	}

	var pasteID string
	for id := range mockClient.Pastes {
		pasteID = id
	}

	// 5. Authorized Recipient (Bob) reads secret -> MUST SUCCEED
	outBuf.Reset()
	ui = NewUI(strings.NewReader(""), &outBuf)
	exitCode = runRead([]string{pasteID, "--config", bobConfig}, ui, mockClient)
	if exitCode != 0 {
		t.Fatalf("Bob read failed with code %d: %s", exitCode, outBuf.String())
	}
	if !strings.Contains(outBuf.String(), secretText) {
		t.Errorf("expected decrypted text '%s' in Bob's output, got: %s", secretText, outBuf.String())
	}

	// 6. Unauthorized Device (Eve) attempts to read the SAME secret -> MUST FAIL (MVP-6)
	outBuf.Reset()
	ui = NewUI(strings.NewReader(""), &outBuf)
	exitCode = runRead([]string{pasteID, "--config", eveConfig}, ui, mockClient)
	if exitCode != 1 {
		t.Fatalf("expected Eve's read to fail with exit code 1, got code %d", exitCode)
	}
	if strings.Contains(outBuf.String(), secretText) {
		t.Errorf("FATAL: Unauthorized device Eve was able to see plaintext!")
	}
	if !strings.Contains(outBuf.String(), "ACCESS DENIED") {
		t.Errorf("expected generic ACCESS DENIED message for Eve, got: %s", outBuf.String())
	}
}

func TestCLI_TamperedCiphertext_FailsCleanly(t *testing.T) {
	tmpDir := t.TempDir()
	bobConfig := filepath.Join(tmpDir, "bob.json")
	mockClient := client.NewMockClient()

	var outBuf bytes.Buffer
	ui := NewUI(strings.NewReader(""), &outBuf)
	_ = runInit([]string{"--config", bobConfig, "--handle", "PV-BOB"}, ui, mockClient)

	// Tampered ciphertext in relay
	mockClient.Pastes["pv_tampered"] = client.PasteCreateRequest{
		Ciphertext: "tampered_corrupted_ciphertext_data",
	}

	outBuf.Reset()
	exitCode := runRead([]string{"pv_tampered", "--config", bobConfig}, ui, mockClient)
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for tampered ciphertext, got %d", exitCode)
	}
	if !strings.Contains(outBuf.String(), "ACCESS DENIED") {
		t.Errorf("expected ACCESS DENIED for tampered ciphertext, got: %s", outBuf.String())
	}
}

func TestCLI_SaveToFile(t *testing.T) {
	tmpDir := t.TempDir()
	bobConfig := filepath.Join(tmpDir, "bob.json")
	savedFilePath := filepath.Join(tmpDir, "decrypted_secret.txt")
	mockClient := client.NewMockClient()

	var outBuf bytes.Buffer
	ui := NewUI(strings.NewReader(""), &outBuf)
	_ = runInit([]string{"--config", bobConfig, "--handle", "PV-BOB"}, ui, mockClient)

	// Bob's public key
	idFile, _ := storage.LoadIdentity(bobConfig)
	ciphertext, _ := crypto.Encrypt([]byte("SAVED_SECRET_TEST"), idFile.PublicKey)
	mockClient.Pastes["pv_save_test"] = client.PasteCreateRequest{
		Ciphertext: string(ciphertext),
	}

	outBuf.Reset()
	exitCode := runRead([]string{"pv_save_test", "--config", bobConfig, "--save", savedFilePath}, ui, mockClient)
	if exitCode != 0 {
		t.Fatalf("runRead with --save failed: %s", outBuf.String())
	}

	savedBytes, err := os.ReadFile(savedFilePath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(savedBytes) != "SAVED_SECRET_TEST" {
		t.Errorf("expected file content 'SAVED_SECRET_TEST', got '%s'", string(savedBytes))
	}
}
