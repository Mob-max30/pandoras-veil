package shell

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

//go:embed static/*
var staticFS embed.FS

// StartShellServer starts the local web server bridging the App Shell UI with local crypto and relay API
func StartShellServer(port int) (string, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return "", fmt.Errorf("failed to load static files: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// Endpoint to get active local identity
	mux.HandleFunc("/api/identity", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id, err := storage.LoadIdentity("")
		if err != nil || id == nil || id.Handle == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"initialized": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"initialized": true,
			"handle":      id.Handle,
			"fingerprint": id.Fingerprint,
			"public_key":  id.PublicKey,
		})
	})

	// Endpoint to initialize/change local handle
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
		if !strings.HasPrefix(strings.ToUpper(handle), "PV-") {
			handle = "PV-" + strings.ToUpper(handle)
		}

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

		// Register with relay if possible
		apiClient := client.NewHTTPClient(client.DefaultRelayURL)
		_, _ = apiClient.RegisterKey(handle, pubKeyStr)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"handle":      handle,
			"fingerprint": fp,
			"public_key":  pubKeyStr,
		})
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
