package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Mob-max30/pandoras-veil/demo-backend/storage"
)

type PasteHandler struct {
	Store *storage.MemoryStore
}

type CreatePasteRequest struct {
	Ciphertext       string `json:"ciphertext"`
	TTLSeconds       int    `json:"ttl_seconds"`
	BurnAfterReading bool   `json:"burn_after_reading"`
}

type CreatePasteResponse struct {
	ID string `json:"id"`
}

type GetPasteResponse struct {
	Ciphertext string `json:"ciphertext"`
}

func generatePasteID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("pv_%s", hex.EncodeToString(b))
}

func (h *PasteHandler) HandleCreatePaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreatePasteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Ciphertext == "" {
		http.Error(w, "ciphertext is required", http.StatusBadRequest)
		return
	}

	pasteID := generatePasteID()
	if err := h.Store.SavePaste(pasteID, req.Ciphertext, req.TTLSeconds, req.BurnAfterReading); err != nil {
		http.Error(w, "Failed to save secret", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreatePasteResponse{ID: pasteID})
}

func (h *PasteHandler) HandleGetPaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 2 || pathParts[1] == "" {
		http.Error(w, "Missing paste ID in path", http.StatusBadRequest)
		return
	}
	id := pathParts[1]

	ciphertext, err := h.Store.GetPaste(id)
	if err != nil {
		if err == storage.ErrNotFound {
			http.Error(w, "Paste not found or already consumed/expired", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetPasteResponse{Ciphertext: ciphertext})
}
