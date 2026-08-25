package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Mob-max30/pandoras-veil/demo-backend/storage"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
)

type KeyHandler struct {
	Store *storage.MemoryStore
}

type RegisterKeyRequest struct {
	Handle    string `json:"handle"`
	PublicKey string `json:"public_key"`
}

type RegisterKeyResponse struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key,omitempty"`
	Fingerprint string `json:"fingerprint"`
}

func (h *KeyHandler) HandleKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req RegisterKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if req.PublicKey == "" {
			http.Error(w, "public_key is required", http.StatusBadRequest)
			return
		}

		fingerprint := crypto.ComputeFingerprint(req.PublicKey)
		handle := strings.TrimSpace(req.Handle)
		if handle == "" {
			handle = fmt.Sprintf("PV-%s", fingerprint)
		}

		if err := h.Store.SaveKey(handle, req.PublicKey, fingerprint); err != nil {
			if err == storage.ErrConflict {
				http.Error(w, "Handle already registered", http.StatusConflict)
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(RegisterKeyResponse{
			Handle:      handle,
			PublicKey:   req.PublicKey,
			Fingerprint: fingerprint,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (h *KeyHandler) HandleGetKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 2 || pathParts[1] == "" {
		http.Error(w, "Missing handle in path", http.StatusBadRequest)
		return
	}
	handle := pathParts[1]

	key, err := h.Store.GetKey(handle)
	if err != nil {
		if err == storage.ErrNotFound {
			http.Error(w, "Key not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RegisterKeyResponse{
		Handle:      key.Handle,
		PublicKey:   key.PublicKey,
		Fingerprint: key.Fingerprint,
	})
}
