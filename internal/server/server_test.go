package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrgomesdev/miner/internal/database"
	"github.com/jrgomesdev/miner/internal/handler"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestRouterUnknownRoutesAndMethods(t *testing.T) {
	log := discardLogger()
	router := NewRouter(log, handler.NewHealth(map[string]database.Pinger{"database": okPinger{}}, log))

	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodGet, "/nao-existe", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}

func TestRecoverPanics(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h := logRequests(log, recoverPanics(log, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"panic":"boom"`)) {
		t.Errorf("panic not logged: %s", buf.String())
	}

	// The handler must keep serving after a panic.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("second request status = %d, want 500", rec.Code)
	}
}

func TestLogRequestsFields(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h := logRequests(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/tea", nil))

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	if rec["method"] != "GET" || rec["path"] != "/tea" || rec["status"] != float64(http.StatusTeapot) {
		t.Errorf("unexpected log fields: %v", rec)
	}
	if _, ok := rec["duration"]; !ok {
		t.Errorf("missing duration field: %v", rec)
	}
}

func TestServeGracefulShutdownWaitsForInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(0, slow, discardLogger(), 2*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.serve(ctx, ln) }()

	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	<-started
	cancel()

	select {
	case resp := <-respCh:
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("in-flight request status = %d, want 200", resp.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("in-flight request failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request did not finish")
	}

	if err := <-done; err != nil {
		t.Errorf("serve() returned %v, want nil", err)
	}
}

func TestServeShutdownTimeoutForcesClose(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	stuck := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	srv := New(0, stuck, log, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.serve(ctx, ln) }()
	go func() { _, _ = http.Get("http://" + ln.Addr().String() + "/") }()

	<-started
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("serve() did not return after the shutdown timeout")
	}
	if !bytes.Contains(buf.Bytes(), []byte("graceful shutdown timed out")) {
		t.Errorf("missing timeout warning in logs: %s", buf.String())
	}
}
