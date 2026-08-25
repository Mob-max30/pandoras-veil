package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Mob-max30/pandoras-veil/demo-backend/handlers"
	"github.com/Mob-max30/pandoras-veil/demo-backend/storage"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store := storage.NewMemoryStore()
	keyHandler := &handlers.KeyHandler{Store: store}
	pasteHandler := &handlers.PasteHandler{Store: store}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	mux.HandleFunc("/keys", keyHandler.HandleKeys)
	mux.HandleFunc("/keys/", keyHandler.HandleGetKey)

	mux.HandleFunc("/paste", pasteHandler.HandleCreatePaste)
	mux.HandleFunc("/paste/", pasteHandler.HandleGetPaste)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("==================================================")
	log.Printf("  PANDORA'S VEIL — ZERO-KNOWLEDGE RELAY SERVER")
	log.Printf("  Listening on http://127.0.0.1:%s (and localhost)", port)
	log.Printf("==================================================")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
