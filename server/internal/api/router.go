package api

import (
	"log/slog"
	"net/http"
	"time"
)

// NewRouter builds the relay's HTTP routing table. Route paths and methods
// mirror the "Backend Relay API Contract" table in the README exactly.
func NewRouter(h *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /keys", h.RegisterKey)
	mux.HandleFunc("GET /keys/{handle}", h.LookupKey)
	mux.HandleFunc("POST /paste", h.UploadPaste)
	mux.HandleFunc("GET /paste/{id}", h.FetchPaste)
	mux.HandleFunc("GET /stream", h.HandleStream)
	mux.HandleFunc("GET /inbox", h.FetchInbox)
	mux.HandleFunc("POST /admin/clear", h.FlushServer)
	mux.HandleFunc("DELETE /admin/clear", h.FlushServer)
	mux.HandleFunc("GET /health", h.Health)

	return withMiddleware(mux, h.Logger)
}

func withMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return requestLogger(logger, recoverer(logger, next))
}

// recoverer converts a panic in any handler into a 500 instead of taking
// down the whole relay process — one malformed request should never crash
// the server other teammates' demo depends on.
func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
