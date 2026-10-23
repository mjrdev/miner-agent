package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	const dbURL = "postgres://u:p@localhost:5432/db"

	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{
			name: "defaults",
			env:  map[string]string{"DATABASE_URL": dbURL},
			want: Config{
				AppEnv:          "development",
				HTTPPort:        8080,
				LogLevel:        slog.LevelInfo,
				DatabaseURL:     dbURL,
				ShutdownTimeout: 10 * time.Second,
			},
		},
		{
			name: "custom values",
			env: map[string]string{
				"APP_ENV":          "staging",
				"HTTP_PORT":        "9090",
				"LOG_LEVEL":        "WARN",
				"DATABASE_URL":     dbURL,
				"SHUTDOWN_TIMEOUT": "3s",
			},
			want: Config{
				AppEnv:          "staging",
				HTTPPort:        9090,
				LogLevel:        slog.LevelWarn,
				DatabaseURL:     dbURL,
				ShutdownTimeout: 3 * time.Second,
			},
		},
		{
			name:    "missing DATABASE_URL",
			env:     map[string]string{},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "invalid HTTP_PORT",
			env:     map[string]string{"DATABASE_URL": dbURL, "HTTP_PORT": "abc"},
			wantErr: "HTTP_PORT",
		},
		{
			name:    "out of range HTTP_PORT",
			env:     map[string]string{"DATABASE_URL": dbURL, "HTTP_PORT": "70000"},
			wantErr: "HTTP_PORT",
		},
		{
			name:    "invalid SHUTDOWN_TIMEOUT",
			env:     map[string]string{"DATABASE_URL": dbURL, "SHUTDOWN_TIMEOUT": "xyz"},
			wantErr: "SHUTDOWN_TIMEOUT",
		},
		{
			name:    "invalid LOG_LEVEL",
			env:     map[string]string{"DATABASE_URL": dbURL, "LOG_LEVEL": "verbose"},
			wantErr: "LOG_LEVEL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := load(func(k string) string { return tt.env[k] })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("load() error = %v, want error mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("load() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
