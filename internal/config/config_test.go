package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	const (
		dbURL = "postgres://u:p@localhost:5432/db"
		mqURL = "amqp://u:p@localhost:5672/"
	)

	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{
			name: "defaults",
			env:  map[string]string{"DATABASE_URL": dbURL, "RABBITMQ_URL": mqURL},
			want: Config{
				AppEnv:          "development",
				HTTPPort:        8080,
				LogLevel:        slog.LevelInfo,
				DatabaseURL:     dbURL,
				RabbitMQURL:     mqURL,
				ShutdownTimeout: 10 * time.Second,
				RunsDir:         "/runs",
			},
		},
		{
			name: "custom values",
			env: map[string]string{
				"APP_ENV":             "staging",
				"HTTP_PORT":           "9090",
				"LOG_LEVEL":           "WARN",
				"DATABASE_URL":        dbURL,
				"RABBITMQ_URL":        mqURL,
				"LOCAL_WEBHOOK_TOKEN": "secret",
				"SHUTDOWN_TIMEOUT":    "3s",
				"RUNS_DIR":            "/data/runs",
			},
			want: Config{
				AppEnv:            "staging",
				HTTPPort:          9090,
				LogLevel:          slog.LevelWarn,
				DatabaseURL:       dbURL,
				RabbitMQURL:       mqURL,
				LocalWebhookToken: "secret",
				ShutdownTimeout:   3 * time.Second,
				RunsDir:           "/data/runs",
			},
		},
		{
			name:    "missing DATABASE_URL",
			env:     map[string]string{"RABBITMQ_URL": mqURL},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "missing RABBITMQ_URL",
			env:     map[string]string{"DATABASE_URL": dbURL},
			wantErr: "RABBITMQ_URL",
		},
		{
			name:    "invalid HTTP_PORT",
			env:     map[string]string{"DATABASE_URL": dbURL, "RABBITMQ_URL": mqURL, "HTTP_PORT": "abc"},
			wantErr: "HTTP_PORT",
		},
		{
			name:    "out of range HTTP_PORT",
			env:     map[string]string{"DATABASE_URL": dbURL, "RABBITMQ_URL": mqURL, "HTTP_PORT": "70000"},
			wantErr: "HTTP_PORT",
		},
		{
			name:    "invalid SHUTDOWN_TIMEOUT",
			env:     map[string]string{"DATABASE_URL": dbURL, "RABBITMQ_URL": mqURL, "SHUTDOWN_TIMEOUT": "xyz"},
			wantErr: "SHUTDOWN_TIMEOUT",
		},
		{
			name:    "invalid LOG_LEVEL",
			env:     map[string]string{"DATABASE_URL": dbURL, "RABBITMQ_URL": mqURL, "LOG_LEVEL": "verbose"},
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
