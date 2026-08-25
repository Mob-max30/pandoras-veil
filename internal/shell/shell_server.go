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
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req struct {
			Handle string `json:"handle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Handle) == "" {
			http.Error(w, "Invalid handle", http.StatusBadRequest)
			return
		}

		handle := strings.TrimSpace(req.Handle)

		// Generate keypair
		identity, err := crypto.GenerateIdentity()
		if err != nil {
			http.Error(w, "Failed to generate identity", http.StatusInternalServerError)
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
			http.Error(w, "Failed to save identity", http.StatusInternalServerError)
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
			http.Error(w, `{"error":"Handle required"}`, http.StatusBadRequest)
			return
		}

		info, err := apiClient.GetKey(handle)
		if err != nil || info == nil || info.PublicKey == "" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": fmt.Sprintf("User '%s' not registered on relay", handle),
			})
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
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req SendPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
			return
		}

		if req.TTL <= 0 {
			req.TTL = 300
		}

		idFile, err := storage.LoadIdentity("")
		if err != nil || idFile == nil || idFile.Handle == "" {
			http.Error(w, `{"error":"Identity not initialized"}`, http.StatusBadRequest)
			return
		}

		// Prepare plaintext bytes (either file or text)
		var plaintextBytes []byte
		if req.File != nil && req.File.Filename != "" && req.File.DataB64 != "" {
			rawBytes, err := base64.StdEncoding.DecodeString(req.File.DataB64)
			if err != nil {
				http.Error(w, `{"error":"Invalid file encoding"}`, http.StatusBadRequest)
				return
			}
			encodedPayload, err := crypto.EncodeFilePayload(req.File.Filename, rawBytes)
			if err != nil {
				http.Error(w, `{"error":"Failed to encode file"}`, http.StatusInternalServerError)
				return
			}
			plaintextBytes = encodedPayload
		} else if req.Text != "" {
			plaintextBytes = []byte(req.Text)
		} else {
			http.Error(w, `{"error":"No content to send"}`, http.StatusBadRequest)
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
				info, err := apiClient.GetKey(trimmed)
				if err == nil && info != nil && info.PublicKey != "" {
					pubKeys = append(pubKeys, info.PublicKey)
					recipientHandles = append(recipientHandles, trimmed)
				}
			}

			if len(pubKeys) == 0 {
				http.Error(w, `{"error":"No registered members found for group"}`, http.StatusBadRequest)
				return
			}

			ciphertext, err := crypto.EncryptMulti(plaintextBytes, pubKeys...)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Encryption failed: %v"}`, err), http.StatusInternalServerError)
				return
			}

			_, err = apiClient.PostGroupChatMessageWithOptions(recipientHandles, idFile.Handle, string(ciphertext), req.TTL, req.Burn)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Relay upload failed: %v"}`, err), http.StatusInternalServerError)
				return
			}
		} else {
			// Direct 1-on-1 message
			targetHandle := strings.TrimSpace(req.Target)
			if targetHandle == "" {
				http.Error(w, `{"error":"Target handle required"}`, http.StatusBadRequest)
				return
			}

			info, err := apiClient.GetKey(targetHandle)
			if err != nil || info == nil || info.PublicKey == "" {
				http.Error(w, fmt.Sprintf(`{"error":"Recipient %s not found on relay"}`, targetHandle), http.StatusBadRequest)
				return
			}

			ciphertext, err := crypto.Encrypt(plaintextBytes, info.PublicKey)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Encryption failed: %v"}`, err), http.StatusInternalServerError)
				return
			}

			_, err = apiClient.PostChatMessageWithOptions(targetHandle, idFile.Handle, string(ciphertext), req.TTL, req.Burn)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Relay upload failed: %v"}`, err), http.StatusInternalServerError)
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
