package shell

import (
	"encoding/base64"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

//go:embed static/*
var staticFS embed.FS

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}

// resolveKey finds a recipient's public key by trying handle variants (case-insensitive & PV- prefix)
func resolveKey(apiClient client.RelayClient, handle string) (*client.KeyInfo, error) {
	trimmed := strings.TrimSpace(handle)
	if trimmed == "" {
		return nil, fmt.Errorf("handle required")
	}

	candidates := []string{
		trimmed,
		strings.ToUpper(trimmed),
		strings.ToLower(trimmed),
		"PV-" + strings.ToUpper(strings.TrimPrefix(trimmed, "PV-")),
		strings.TrimPrefix(trimmed, "PV-"),
		strings.TrimPrefix(strings.ToUpper(trimmed), "PV-"),
	}

	seen := make(map[string]bool)
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		info, err := apiClient.GetKey(c)
		if err == nil && info != nil && info.PublicKey != "" {
			return info, nil
		}
	}

	return nil, fmt.Errorf("user '%s' not registered on relay", handle)
}

// StartShellServer starts the local web server bridging the App Shell UI with local crypto and relay API
func StartShellServer(port int, apiClient client.RelayClient) (string, error) {
	if apiClient == nil {
		apiClient = client.NewHTTPClient(client.DefaultRelayURL)
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return "", fmt.Errorf("failed to load static files: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// 1. Endpoint to get active local identity
	mux.HandleFunc("/api/identity", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id, err := storage.LoadIdentity("")
		if err != nil || id == nil || id.Handle == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"initialized": false})
			return
		}

		// Ensure handle is actively registered on cloud relay
		go func(h, pk string) {
			_, _ = apiClient.RegisterKey(h, pk)
		}(id.Handle, id.PublicKey)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"initialized": true,
			"handle":      id.Handle,
			"fingerprint": id.Fingerprint,
			"public_key":  id.PublicKey,
		})
	})

	// 2. Endpoint to initialize/change local handle
	mux.HandleFunc("/api/init", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req struct {
			Handle string `json:"handle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Handle) == "" {
			writeJSONError(w, http.StatusBadRequest, "Invalid handle")
			return
		}

		handle := strings.TrimSpace(req.Handle)

		// Generate keypair
		identity, err := crypto.GenerateIdentity()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to generate identity")
			return
		}

		pubKeyStr := crypto.GetPublicKey(identity)
		fp := crypto.ComputeFingerprint(pubKeyStr)

		idFile := &storage.IdentityFile{
			Handle:      handle,
			PublicKey:   pubKeyStr,
			PrivateKey:  identity.String(),
			Fingerprint: fp,
		}

		if err := storage.SaveIdentity("", idFile); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to save identity")
			return
		}

		// Register with relay
		_, _ = apiClient.RegisterKey(handle, pubKeyStr)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"handle":      handle,
			"fingerprint": fp,
			"public_key":  pubKeyStr,
		})
	})

	// 3. Endpoint to lookup recipient info
	mux.HandleFunc("/api/lookup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handle := strings.TrimSpace(r.URL.Query().Get("handle"))
		if handle == "" {
			writeJSONError(w, http.StatusBadRequest, "Handle required")
			return
		}

		info, err := resolveKey(apiClient, handle)
		if err != nil || info == nil || info.PublicKey == "" {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("User '%s' not registered on relay", handle))
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"handle":      info.Handle,
			"publicKey":   info.PublicKey,
			"fingerprint": info.Fingerprint,
		})
	})

	// 4. Endpoint to send encrypted message or file
	type FileData struct {
		Filename string `json:"filename"`
		DataB64  string `json:"dataB64"`
	}

	type SendPayload struct {
		Target       string    `json:"target"`
		IsGroup      bool      `json:"isGroup"`
		GroupMembers []string  `json:"groupMembers"`
		Text         string    `json:"text"`
		File         *FileData `json:"file,omitempty"`
		TTL          int       `json:"ttl"`
		Burn         bool      `json:"burn"`
	}

	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req SendPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		if req.TTL <= 0 {
			req.TTL = 300
		}

		idFile, err := storage.LoadIdentity("")
		if err != nil || idFile == nil || idFile.Handle == "" {
			writeJSONError(w, http.StatusBadRequest, "Identity not initialized")
			return
		}

		// Prepare plaintext bytes (either file or text)
		var plaintextBytes []byte
		if req.File != nil && req.File.Filename != "" && req.File.DataB64 != "" {
			rawBytes, err := base64.StdEncoding.DecodeString(req.File.DataB64)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "Invalid file encoding")
				return
			}
			if len(rawBytes) > 2*1024*1024 {
				writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("File exceeds 2MB limit (%d KB)", len(rawBytes)/1024))
				return
			}
			encodedPayload, err := crypto.EncodeFilePayload(req.File.Filename, rawBytes)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "Failed to encode file payload")
				return
			}
			plaintextBytes = encodedPayload
		} else if req.Text != "" {
			plaintextBytes = []byte(req.Text)
		} else {
			writeJSONError(w, http.StatusBadRequest, "No text or file to send")
			return
		}

		if req.IsGroup || strings.HasPrefix(req.Target, "#") {
			// Multi-recipient group chat
			members := req.GroupMembers
			if len(members) == 0 {
				members = []string{req.Target}
			}
			var pubKeys []string
			var recipientHandles []string
			for _, m := range members {
				trimmed := strings.TrimSpace(m)
				if trimmed == "" || strings.EqualFold(trimmed, idFile.Handle) {
					continue
				}
				info, err := resolveKey(apiClient, trimmed)
				if err == nil && info != nil && info.PublicKey != "" {
					pubKeys = append(pubKeys, info.PublicKey)
					recipientHandles = append(recipientHandles, trimmed)
				}
			}

			if len(pubKeys) == 0 {
				writeJSONError(w, http.StatusBadRequest, "No registered members found for group")
				return
			}

			ciphertext, err := crypto.EncryptMulti(plaintextBytes, pubKeys...)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Encryption failed: %v", err))
				return
			}

			_, err = apiClient.PostGroupChatMessageWithOptions(recipientHandles, idFile.Handle, string(ciphertext), req.TTL, req.Burn)
			if err != nil {
				writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("Relay upload failed: %v", err))
				return
			}
		} else {
			// Direct 1-on-1 message
			targetHandle := strings.TrimSpace(req.Target)
			if targetHandle == "" {
				writeJSONError(w, http.StatusBadRequest, "Target handle required")
				return
			}

			info, err := resolveKey(apiClient, targetHandle)
			if err != nil || info == nil || info.PublicKey == "" {
				writeJSONError(w, http.StatusNotFound, fmt.Sprintf("Recipient '%s' not registered on relay", targetHandle))
				return
			}

			ciphertext, err := crypto.Encrypt(plaintextBytes, info.PublicKey)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Encryption failed: %v", err))
				return
			}

			_, err = apiClient.PostChatMessageWithOptions(info.Handle, idFile.Handle, string(ciphertext), req.TTL, req.Burn)
			if err != nil {
				writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("Relay upload failed: %v", err))
				return
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})

	// 5. Endpoint to stream incoming decrypted messages via SSE
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		idFile, err := storage.LoadIdentity("")
		if err != nil || idFile == nil || idFile.PrivateKey == "" {
			http.Error(w, "Identity uninitialized", http.StatusBadRequest)
			return
		}

		devIdentity, err := crypto.ParseIdentity(idFile.PrivateKey)
		if err != nil {
			http.Error(w, fmt.Sprintf("Identity error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		stopCh := make(chan struct{})
		defer close(stopCh)

		go func() {
			_ = apiClient.ListenStream(idFile.Handle, func(event client.StreamEvent) {
				plaintext, err := crypto.Decrypt([]byte(event.Ciphertext), devIdentity)
				if err != nil {
					return
				}

				timestamp := time.Now().Format("15:04")
				filename, fileData, isFile := crypto.DecodeFilePayload(plaintext)

				var payload map[string]interface{}
				if isFile {
					_ = os.MkdirAll("./downloads", 0755)
					savePath := filepath.Join("./downloads", filename)
					_ = os.WriteFile(savePath, fileData, 0600)

					payload = map[string]interface{}{
						"sender":    event.Sender,
						"isFile":    true,
						"filename":  filename,
						"fileSize":  len(fileData),
						"savePath":  savePath,
						"timestamp": timestamp,
					}
				} else {
					payload = map[string]interface{}{
						"sender":    event.Sender,
						"text":      string(plaintext),
						"timestamp": timestamp,
					}
				}

				dataBytes, _ := json.Marshal(payload)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", string(dataBytes))
				flusher.Flush()
			}, stopCh)
		}()

		<-r.Context().Done()
	})

	// 6. Endpoint to fetch offline queued messages for a sender
	mux.HandleFunc("/api/inbox", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sender := strings.TrimSpace(r.URL.Query().Get("sender"))

		idFile, err := storage.LoadIdentity("")
		if err != nil || idFile == nil || idFile.PrivateKey == "" {
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		devIdentity, err := crypto.ParseIdentity(idFile.PrivateKey)
		if err != nil {
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		inboxMsgs, err := apiClient.FetchInbox(idFile.Handle, sender)
		if err != nil {
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		var results []map[string]interface{}
		for _, msg := range inboxMsgs {
			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				continue
			}

			timestamp := time.Now().Format("15:04")
			filename, fileData, isFile := crypto.DecodeFilePayload(plaintext)

			senderName := msg.Sender
			if senderName == "" {
				senderName = sender
			}

			if isFile {
				_ = os.MkdirAll("./downloads", 0755)
				savePath := filepath.Join("./downloads", filename)
				_ = os.WriteFile(savePath, fileData, 0600)

				results = append(results, map[string]interface{}{
					"sender":    senderName,
					"isFile":    true,
					"filename":  filename,
					"fileSize":  len(fileData),
					"savePath":  savePath,
					"timestamp": timestamp,
				})
			} else {
				results = append(results, map[string]interface{}{
					"sender":    senderName,
					"text":      string(plaintext),
					"timestamp": timestamp,
				})
			}
		}

		if results == nil {
			results = []map[string]interface{}{}
		}
		json.NewEncoder(w).Encode(results)
	})

	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	go func() {
		_ = server.ListenAndServe()
	}()

	return "http://" + serverAddr, nil
}
