// Command server runs the Pandora's Veil backend relay.
//
// Owner: Pavan (Backend Relay Lead) — server/
//
// The relay is deliberately dumb: it never sees plaintext, never sees a
// private key, and does not implement or call any cryptography. It stores
// opaque ciphertext blobs and public-key strings, and enforces two pieces
// of lifecycle policy — TTL expiry and atomic burn-after-reading — via
// Redis. Everything security-relevant happens on the client (see
// internal/crypto, owned by Pranav).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mob-max30/pandoras-veil/server/internal/api"
	"github.com/Mob-max30/pandoras-veil/server/internal/config"
	"github.com/Mob-max30/pandoras-veil/server/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Error("failed to connect to redis", "error", err, "addr", cfg.RedisAddr)
		os.Exit(1)
	}
	defer st.Close()

	handlers := api.New(
		st,
		api.TTLPolicy{Default: cfg.DefaultTTL, Min: cfg.MinTTL, Max: cfg.MaxTTL},
		cfg.MaxCiphertextBytes,
		logger,
	)
	router := api.NewRouter(handlers)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("relay listening", "port", cfg.Port, "redis_addr", cfg.RedisAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received, draining in-flight requests")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGracePeriod)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
