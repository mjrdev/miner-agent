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
)

type fakePinger func(ctx context.Context) error

func (f fakePinger) Ping(ctx context.Context) error { return f(ctx) }

func newTestHealth(p fakePinger) *Health {
	h := NewHealth(p, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	h.timeout = 50 * time.Millisecond
	return h
}

func TestLiveness(t *testing.T) {
	// Liveness must not depend on the database, so the pinger always fails.
	h := newTestHealth(func(context.Context) error { return errors.New("down") })

	rec := httptest.NewRecorder()
	h.Liveness(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assertResponse(t, rec, http.StatusOK, `{"status":"ok"}`)
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		ping       fakePinger
		wantStatus int
		wantBody   string
	}{
		{
			name:       "database reachable",
			ping:       func(context.Context) error { return nil },
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ready","checks":{"database":"ok"}}`,
		},
		{
			name:       "database error",
			ping:       func(context.Context) error { return errors.New("dial tcp: password=secret refused") },
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"not_ready","checks":{"database":"unavailable"}}`,
		},
		{
			name: "database timeout",
			ping: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"not_ready","checks":{"database":"unavailable"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestHealth(tt.ping).Readiness(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
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
