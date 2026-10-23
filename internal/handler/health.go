// Package handler contains the HTTP handlers exposed by the service.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jrgomesdev/miner/internal/database"
)

const readinessTimeout = 2 * time.Second

// Health serves the liveness and readiness endpoints.
type Health struct {
	db      database.Pinger
	log     *slog.Logger
	timeout time.Duration
}

// NewHealth returns health handlers that check db for readiness.
func NewHealth(db database.Pinger, log *slog.Logger) *Health {
	return &Health{db: db, log: log, timeout: readinessTimeout}
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Liveness reports that the process is up, regardless of its dependencies.
func (h *Health) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// Readiness reports whether the service can serve traffic, which requires a
// reachable database.
func (h *Health) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.log.WarnContext(r.Context(), "readiness check failed", "check", "database", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{
			Status: "not_ready",
			Checks: map[string]string{"database": "unavailable"},
		})
		return
	}

	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ready",
		Checks: map[string]string{"database": "ok"},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
