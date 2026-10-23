package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jrgomesdev/miner/internal/database"
)

type fakePinger func(ctx context.Context) error

func (f fakePinger) Ping(ctx context.Context) error { return f(ctx) }

func ok(context.Context) error { return nil }

func newTestHealth(db, broker fakePinger) *Health {
	h := NewHealth(
		map[string]database.Pinger{"database": db, "broker": broker},
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)
	h.timeout = 50 * time.Millisecond
	return h
}

func TestLiveness(t *testing.T) {
	// Liveness must not depend on any dependency, so the pingers always fail.
	down := func(context.Context) error { return errors.New("down") }
	h := newTestHealth(down, down)

	rec := httptest.NewRecorder()
	h.Liveness(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assertResponse(t, rec, http.StatusOK, `{"status":"ok"}`)
}

func TestReadiness(t *testing.T) {
	timeout := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	tests := []struct {
		name       string
		db, broker fakePinger
		wantStatus int
		wantBody   string
	}{
		{
			name:       "all reachable",
			db:         ok,
			broker:     ok,
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ready","checks":{"broker":"ok","database":"ok"}}`,
		},
		{
			name:       "database error",
			db:         func(context.Context) error { return errors.New("dial tcp: password=secret refused") },
			broker:     ok,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"not_ready","checks":{"broker":"ok","database":"unavailable"}}`,
		},
		{
			name:       "database timeout",
			db:         timeout,
			broker:     ok,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"not_ready","checks":{"broker":"ok","database":"unavailable"}}`,
		},
		{
			name:       "broker unavailable",
			db:         ok,
			broker:     func(context.Context) error { return errors.New("broker: not connected") },
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"not_ready","checks":{"broker":"unavailable","database":"ok"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestHealth(tt.db, tt.broker).Readiness(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			assertResponse(t, rec, tt.wantStatus, tt.wantBody)
		})
	}
}

func assertResponse(t *testing.T, rec *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d", rec.Code, status)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != body {
		t.Errorf("body = %s, want %s", got, body)
	}
}
