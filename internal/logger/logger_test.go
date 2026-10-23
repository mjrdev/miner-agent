package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewFiltersByLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelWarn)

	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")

	out := buf.String()
	if strings.Contains(out, "debug message") || strings.Contains(out, "info message") {
		t.Fatalf("records below warn were written: %s", out)
	}
	if !strings.Contains(out, "warn message") {
		t.Fatalf("warn record missing: %s", out)
	}
}

func TestNewWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo).Info("hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not JSON: %v (%s)", err, buf.String())
	}
	for _, key := range []string{"time", "level", "msg"} {
		if _, ok := rec[key]; !ok {
			t.Errorf("missing field %q in %v", key, rec)
		}
	}
}
