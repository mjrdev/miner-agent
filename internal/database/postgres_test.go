package database

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestConnectUnreachableTimesOut(t *testing.T) {
	const password = "s3cr3t-password"
	// Port 1 on loopback refuses connections immediately.
	url := "postgres://miner:" + password + "@127.0.0.1:1/miner?sslmode=disable&connect_timeout=1"
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	start := time.Now()
	pool, err := connect(context.Background(), url, log, 500*time.Millisecond, 100*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("connect() succeeded against an unreachable database")
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("connect() gave up after %s, before the timeout", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("connect() took %s, far beyond the timeout", elapsed)
	}
	if strings.Contains(err.Error(), password) {
		t.Errorf("error exposes the password: %v", err)
	}
}

func TestConnectInvalidURLHidesValue(t *testing.T) {
	const url = "postgres://miner:s3cr3t@::bad::"
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	_, err := connect(context.Background(), url, log, time.Second, 100*time.Millisecond)
	if err == nil {
		t.Fatal("connect() accepted an invalid URL")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error exposes the URL: %v", err)
	}
}
