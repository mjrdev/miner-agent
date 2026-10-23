// Package logger builds the structured JSON logger used by the service.
package logger

import (
	"io"
	"log/slog"
)

// New returns a JSON logger writing to w that drops records below level.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
