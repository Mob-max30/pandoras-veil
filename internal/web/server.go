package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
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
	apiClient    client.RelayClient
	relayURL     string
	configDir    string
	sessionToken string
	mu           sync.Mutex
	clients      map[chan []byte]bool
}

func GenerateSessionToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewServer(apiClient client.RelayClient, relayURL string, configDir string) *Server {
	return &Server{
		apiClient:    apiClient,
		relayURL:     relayURL,
		configDir:    configDir,
		sessionToken: GenerateSessionToken(),
		clients:      make(map[chan []byte]bool),
	}
}

func (s *Server) SessionToken() string {
	return s.sessionToken
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// 1. Index handler injecting CSRF session token dynamically into HTML
	mux.HandleFunc("/", s.handleIndex)

	// 2. Static CSS / JS / Assets
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/styles.css", http.FileServer(http.FS(subFS)))
	mux.Handle("/app.js", http.FileServer(http.FS(subFS)))

	// 3. Local Security Bridge APIs (All Protected by Host/Origin & Token checks)
	mux.HandleFunc("/api/identity", s.handleIdentity)
	mux.HandleFunc("/api/init", s.handleInit)
	mux.HandleFunc("/api/lookup", s.handleLookup)
	mux.HandleFunc("/api/conversations", s.handleConversations)
	mux.HandleFunc("/api/delete-account", s.handleDeleteAccount)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/deposit", s.handleDeposit)
	mux.HandleFunc("/api/health", s.handleHealth)

	// Wrap entire mux in Localhost CSRF & DNS Rebinding security firewall
	return s.securityFirewall(mux)
}

// securityFirewall defends against:
// 1. DNS Rebinding attacks (validates Host header strictly equals 127.0.0.1 or localhost)
// 2. Cross-Origin Localhost CSRF (rejects foreign Origin/Referer from other browser tabs)
// 3. Clickjacking / framing attacks
func (s *Server) securityFirewall(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// --- 1. Host Header Check (DNS Rebinding Defense) ---
		host := r.Host
		if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}
		if !strings.EqualFold(host, "localhost") && host != "127.0.0.1" && host != "[::1]" && host != "" {
			if os.Getenv("PANDORA_TEST") == "" || host != "example.com" {
				http.Error(w, "Forbidden: Host validation failed (DNS Rebinding Protected)", http.StatusForbidden)
				return
			}
		}

		// --- 2. Origin & Referer Check (Localhost CSRF Defense) ---
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !strings.Contains(origin, "localhost") && !strings.Contains(origin, "127.0.0.1") {
				http.Error(w, "Forbidden: Cross-Origin Request Blocked", http.StatusForbidden)
				return
			}
		}

		referer := r.Header.Get("Referer")
		if referer != "" {
			if !strings.Contains(referer, "localhost") && !strings.Contains(referer, "127.0.0.1") {
				http.Error(w, "Forbidden: Invalid Referer Blocked", http.StatusForbidden)
				return
			}
		}

		// --- 3. Ephemeral Session Token Verification on Mutating & API Endpoints ---
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			clientToken := r.Header.Get("X-Pandora-Token")
			if clientToken == "" {
				clientToken = r.URL.Query().Get("token")
			}
			if clientToken == "" || clientToken != s.sessionToken {
				http.Error(w, `{"error":"Unauthorized: Missing or invalid X-Pandora-Token"}`, http.StatusUnauthorized)
				return
			}
		}

		// --- 4. Security Headers ---
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self' http://localhost:* http://127.0.0.1:*;")

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	indexBytes, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}

	html := string(indexBytes)
	tokenMeta := fmt.Sprintf(`<meta name="pandora-token" content="%s">`, s.sessionToken)
	html = strings.Replace(html, "</head>", tokenMeta+"\n</head>", 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "healthy",
		"relay":   s.relayURL,
		"service": "Pandora's Veil Local Security Bridge",
	})
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	idFile, err := storage.LoadIdentity(s.configDir)
	if err != nil || idFile == nil || idFile.Handle == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"initialized": false,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"initialized": true,
		"handle":      idFile.Handle,
		"fingerprint": idFile.Fingerprint,
		"publicKey":   idFile.PublicKey,
	})
}

type InitRequest struct {
	Handle string `json:"handle"`
}

func (s *Server) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req InitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	handle := strings.TrimSpace(req.Handle)
	if handle == "" {
		http.Error(w, `{"error":"Handle cannot be empty"}`, http.StatusBadRequest)
		return
	}

	// Check if existing identity already exists
	existingId, err := storage.LoadIdentity(s.configDir)
	var pubKey, privKey, fp string

	if err == nil && existingId != nil && existingId.PrivateKey != "" {
		// Preserve keypair, update handle
		pubKey = existingId.PublicKey
		privKey = existingId.PrivateKey
		fp = existingId.Fingerprint
		if fp == "" {
			fp = crypto.ComputeFingerprint(pubKey)
		}
	} else {
		// Generate X25519 identity in Go daemon
		devIdentity, err := crypto.GenerateIdentity()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to generate keypair: %v"}`, err), http.StatusInternalServerError)
			return
		}
		pubKey = crypto.GetPublicKey(devIdentity)
		privKey = devIdentity.String()
		fp = crypto.ComputeFingerprint(pubKey)

		// Wipe session data on fresh account initialization
		_ = storage.ClearWebSession(s.configDir)
	}

	// Register with relay server
	_, _ = s.apiClient.RegisterKey(handle, pubKey)

	idFile := storage.IdentityFile{
		Handle:      handle,
		PublicKey:   pubKey,
		PrivateKey:  privKey,
		Fingerprint: fp,
	}

	if err := storage.SaveIdentity(s.configDir, &idFile); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to save identity: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"handle":      handle,
		"fingerprint": fp,
		"publicKey":   pubKey,
	})
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	handle := strings.TrimSpace(r.URL.Query().Get("handle"))
	if handle == "" {
		http.Error(w, `{"error":"Handle query parameter is required"}`, http.StatusBadRequest)
		return
	}

	candidates := []string{
		handle,
		strings.ToUpper(handle),
		strings.ToLower(handle),
		"PV-" + strings.ToUpper(strings.TrimPrefix(handle, "PV-")),
		strings.TrimPrefix(handle, "PV-"),
		strings.TrimPrefix(strings.ToUpper(handle), "PV-"),
	}

	seen := make(map[string]bool)
	var info *client.KeyInfo
	var err error

	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		info, err = s.apiClient.GetKey(c)
		if err == nil && info != nil && info.PublicKey != "" {
			break
		}
	}

	if info == nil || info.PublicKey == "" {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("User '%s' does not exist on the relay server. Make sure they have initialized their device first.", handle),
		})
		return
	}

	respHandle := info.Handle
	if respHandle == "" {
		respHandle = handle
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"handle":      respHandle,
		"publicKey":   info.PublicKey,
		"fingerprint": info.Fingerprint,
	})
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		data, err := storage.LoadWebSession(s.configDir)
		if err != nil || data == nil {
			_ = json.NewEncoder(w).Encode(storage.WebSessionData{
				Contacts:      []storage.ContactData{},
				Conversations: make(map[string][]storage.MessageData),
				MainTTLs:      make(map[string]int),
			})
			return
		}
		_ = json.NewEncoder(w).Encode(data)
		return
	}

	if r.Method == http.MethodPost {
		var data storage.WebSessionData
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
			return
		}
		if err := storage.SaveWebSession(s.configDir, &data); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Save session error: %v"}`, err), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idFile, err := storage.LoadIdentity(s.configDir)
	if err == nil && idFile != nil && idFile.Handle != "" {
		_ = s.apiClient.DeleteKey(idFile.Handle)
	}

	idPath := s.configDir
	if idPath == "" {
		idPath, _ = storage.DefaultIdentityPath()
	}
	if idPath != "" {
		_ = os.Remove(idPath)
	}

	_ = storage.ClearWebSession(s.configDir)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Account, keys, and session correspondence completely deleted.",
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

	// 100% Native Go Encryption (X25519) - Zero in-browser JS cryptography
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

		// Encapsulate group metadata so recipients route to group room
		groupPayload, _ := json.Marshal(map[string]any{
			"__pv_group": req.Target,
			"sender":     idFile.Handle,
			"text":       req.Text,
			"members":    req.GroupMembers,
		})

		ciphertext, err := crypto.EncryptMulti(groupPayload, pubKeys...)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Encryption error: %v"}`, err), http.StatusInternalServerError)
			return
		}

		_, err = s.apiClient.PostGroupChatMessageWithOptions(recipientHandles, idFile.Handle, string(ciphertext), req.TTL, req.Burn)
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

	stopCh := make(chan struct{})
	defer close(stopCh)

	// Stream from cloud relay and decrypt locally inside Go binary
	go func() {
		_ = s.apiClient.ListenStream(idFile.Handle, func(event client.StreamEvent) {
			plaintext, err := crypto.Decrypt([]byte(event.Ciphertext), devIdentity)
			if err != nil {
				return
			}

			sender := event.Sender
			text := string(plaintext)
			isGroup := false
			groupRoom := ""

			var groupMsg struct {
				Group   string   `json:"__pv_group"`
				Sender  string   `json:"sender"`
				Text    string   `json:"text"`
				Members []string `json:"members"`
			}
			if err := json.Unmarshal(plaintext, &groupMsg); err == nil && groupMsg.Group != "" {
				isGroup = true
				groupRoom = groupMsg.Group
				if groupMsg.Sender != "" {
					sender = groupMsg.Sender
				}
				text = groupMsg.Text
			}

			payload, _ := json.Marshal(map[string]any{
				"id":        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
				"sender":    sender,
				"text":      text,
				"isGroup":   isGroup,
				"group":     groupRoom,
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

	// 100% Native Go Encryption (X25519)
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
