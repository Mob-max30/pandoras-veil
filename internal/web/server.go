package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	apiClient client.RelayClient
	relayURL  string
	configDir string
	mu        sync.Mutex
	clients   map[chan []byte]bool
}

func NewServer(apiClient client.RelayClient, relayURL string, configDir string) *Server {
	return &Server{
		apiClient: apiClient,
		relayURL:  relayURL,
		configDir: configDir,
		clients:   make(map[chan []byte]bool),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Static assets embedded
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	// API Routes
	mux.HandleFunc("/api/identity", s.handleIdentity)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/deposit", s.handleDeposit)
	mux.HandleFunc("/api/health", s.handleHealth)

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idFile, err := storage.LoadIdentity(s.configDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"handle":      idFile.Handle,
		"fingerprint": idFile.Fingerprint,
		"publicKey":   idFile.PublicKey,
	})
}

type SendRequest struct {
	Target       string   `json:"target"`
	IsGroup      bool     `json:"isGroup"`
	GroupMembers []string `json:"groupMembers"`
	Text         string   `json:"text"`
	TTL          int      `json:"ttl"`
	Burn         bool     `json:"burn"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	idFile, err := storage.LoadIdentity(s.configDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	// Resolve public key(s)
	if req.IsGroup && len(req.GroupMembers) > 0 {
		var pubKeys []string
		var recipientHandles []string
		for _, member := range req.GroupMembers {
			trimmed := strings.TrimSpace(member)
			if trimmed == "" || strings.EqualFold(trimmed, idFile.Handle) {
				continue
			}
			info, err := s.apiClient.GetKey(trimmed)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Failed to resolve member %s: %v"}`, trimmed, err), http.StatusBadRequest)
				return
			}
			pubKeys = append(pubKeys, info.PublicKey)
			recipientHandles = append(recipientHandles, trimmed)
		}

		if len(pubKeys) == 0 {
			http.Error(w, `{"error":"No valid recipients for group chat"}`, http.StatusBadRequest)
			return
		}

		ciphertext, err := crypto.EncryptMulti([]byte(req.Text), pubKeys...)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Encryption error: %v"}`, err), http.StatusInternalServerError)
			return
		}

		_, err = s.apiClient.PostGroupChatMessage(recipientHandles, idFile.Handle, string(ciphertext))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Relay error: %v"}`, err), http.StatusInternalServerError)
			return
		}
	} else {
		targetHandle := strings.TrimSpace(req.Target)
		if targetHandle == "" {
			http.Error(w, `{"error":"Target recipient required"}`, http.StatusBadRequest)
			return
		}

		info, err := s.apiClient.GetKey(targetHandle)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to resolve target %s: %v"}`, targetHandle, err), http.StatusBadRequest)
			return
		}

		ciphertext, err := crypto.Encrypt([]byte(req.Text), info.PublicKey)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Encryption error: %v"}`, err), http.StatusInternalServerError)
			return
		}

		_, err = s.apiClient.PostChatMessage(targetHandle, idFile.Handle, string(ciphertext))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Relay error: %v"}`, err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	idFile, err := storage.LoadIdentity(s.configDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	devIdentity, err := crypto.ParseIdentity(idFile.PrivateKey)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stopCh := make(chan struct{})
	defer close(stopCh)

	// Stream from cloud relay and decrypt locally
	go func() {
		_ = s.apiClient.ListenStream(idFile.Handle, func(event client.StreamEvent) {
			plaintext, err := crypto.Decrypt([]byte(event.Ciphertext), devIdentity)
			if err != nil {
				return
			}

			payload, _ := json.Marshal(map[string]string{
				"sender":    event.Sender,
				"text":      string(plaintext),
				"timestamp": time.Now().Format("15:04"),
			})

			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(payload))
			flusher.Flush()
		}, stopCh)
	}()

	// Wait until client disconnects
	<-r.Context().Done()
}

type DepositRequest struct {
	Recipient string `json:"recipient"`
	Secret    string `json:"secret"`
	TTL       int    `json:"ttl"`
}

func (s *Server) handleDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	idFile, err := storage.LoadIdentity(s.configDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	target := strings.TrimSpace(req.Recipient)
	if target == "" {
		target = idFile.Handle
	}

	info, err := s.apiClient.GetKey(target)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to resolve recipient: %v"}`, err), http.StatusBadRequest)
		return
	}

	ciphertext, err := crypto.Encrypt([]byte(req.Secret), info.PublicKey)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Encryption failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = 300
	}

	pasteID, err := s.apiClient.PostPaste(string(ciphertext), ttl, true)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Relay deposit error: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":         pasteID,
		"status":     "deposited",
		"ttlSeconds": fmt.Sprintf("%d", ttl),
	})
}
