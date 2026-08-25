package crypto

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"

	"filippo.io/age"
)

// Encrypt encrypts a plaintext payload using age recipient-based encryption
// targeting the provided recipient public key string (e.g., "age1...").
func Encrypt(plaintext []byte, recipientPublicKey string) ([]byte, error) {
	recipient, err := age.ParseX25519Recipient(recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient public key: %w", err)
	}

	var out bytes.Buffer
	w, err := age.Encrypt(&out, recipient)
	if err != nil {
		return nil, fmt.Errorf("failed to create age encryptor: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("failed to write plaintext to age encryptor: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize age encryption: %w", err)
	}

	return out.Bytes(), nil
}

// EncryptMulti encrypts a plaintext payload using age recipient-based encryption
// targeting multiple recipient public key strings (e.g. for group chat).
func EncryptMulti(plaintext []byte, recipientPublicKeys ...string) ([]byte, error) {
	if len(recipientPublicKeys) == 0 {
		return nil, fmt.Errorf("at least one recipient public key is required")
	}

	recipients := make([]age.Recipient, 0, len(recipientPublicKeys))
	for _, pubKeyStr := range recipientPublicKeys {
		r, err := age.ParseX25519Recipient(pubKeyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient public key '%s': %w", pubKeyStr, err)
		}
		recipients = append(recipients, r)
	}

	var out bytes.Buffer
	w, err := age.Encrypt(&out, recipients...)
	if err != nil {
		return nil, fmt.Errorf("failed to create age encryptor: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("failed to write plaintext to age encryptor: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize age encryption: %w", err)
	}

	return out.Bytes(), nil
}

// FilePayload represents an encapsulated binary file payload for secure sharing.
type FilePayload struct {
	IsFile    bool   `json:"is_file"`
	IsFileAlt bool   `json:"__pv_file"`
	Filename  string `json:"filename"`
	NameAlt   string `json:"name"`
	Size      string `json:"size"`
	Type      string `json:"type"`
	DataB64   string `json:"data_b64"`
	DataURL   string `json:"data"`
}

// EncodeFilePayload serializes a binary file and its filename into an age-encryptable byte slice.
func EncodeFilePayload(filename string, data []byte) ([]byte, error) {
	fp := FilePayload{
		IsFile:   true,
		Filename: filepath.Base(filename),
		DataB64:  base64.StdEncoding.EncodeToString(data),
	}
	return json.Marshal(fp)
}

// EncryptFilePayload encrypts a binary file payload for a single recipient.
func EncryptFilePayload(filename string, data []byte, recipientPublicKey string) ([]byte, error) {
	encoded, err := EncodeFilePayload(filename, data)
	if err != nil {
		return nil, fmt.Errorf("failed to encode file payload: %w", err)
	}
	return Encrypt(encoded, recipientPublicKey)
}

// EncryptFilePayloadMulti encrypts a binary file payload for multiple recipients (e.g. group chat).
func EncryptFilePayloadMulti(filename string, data []byte, recipientPublicKeys ...string) ([]byte, error) {
	encoded, err := EncodeFilePayload(filename, data)
	if err != nil {
		return nil, fmt.Errorf("failed to encode file payload: %w", err)
	}
	return EncryptMulti(encoded, recipientPublicKeys...)
}


