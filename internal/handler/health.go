// Package handler contains the HTTP handlers exposed by the service.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jrgomesdev/miner/internal/database"
)

const readinessTimeout = 2 * time.Second

// Health serves the liveness and readiness endpoints.
type Health struct {
	checks  map[string]database.Pinger
	log     *slog.Logger
	timeout time.Duration
}

// NewHealth returns health handlers whose readiness requires every dependency
// in checks, keyed by the name reported in the response, to answer a ping.
func NewHealth(checks map[string]database.Pinger, log *slog.Logger) *Health {
	return &Health{checks: checks, log: log, timeout: readinessTimeout}
}

// Register adds the health routes to mux.
func (h *Health) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.Liveness)
	mux.HandleFunc("GET /readyz", h.Readiness)
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Liveness reports that the process is up, regardless of its dependencies.
func (h *Health) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// Readiness reports whether the service can serve traffic, which requires
// every dependency to answer a ping. The dependencies are checked in parallel.
func (h *Health) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		ready  = true
		checks = make(map[string]string, len(h.checks))
	)
	for name, p := range h.checks {
		wg.Go(func() {
			state := "ok"
			if err := p.Ping(ctx); err != nil {
				h.log.WarnContext(r.Context(), "readiness check failed", "check", name, "error", err)
				state = "unavailable"
			}
			mu.Lock()
			defer mu.Unlock()
			checks[name] = state
			ready = ready && state == "ok"
		})
	}
	wg.Wait()

	if !ready {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "not_ready", Checks: checks})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ready", Checks: checks})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
