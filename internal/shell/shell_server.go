package shell

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
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

	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := http.Get("http://" + serverAddr)
	if err == nil {
		listener.Body.Close()
		// Server already running
		return "http://" + serverAddr, nil
	}

	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	go func() {
		_ = server.ListenAndServe()
	}()

	return "http://" + serverAddr, nil
}
