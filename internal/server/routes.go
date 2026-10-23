package server

import (
	"log/slog"
	"net/http"

	"github.com/jrgomesdev/miner/internal/handler"
)

// NewRouter registers every route and wraps them with the common middleware.
func NewRouter(health *handler.Health, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Liveness)
	mux.HandleFunc("GET /readyz", health.Readiness)

	// Logging is outermost so recovered panics are logged with their 500 status.
	return logRequests(log, recoverPanics(log, mux))
}
