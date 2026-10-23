// Package config loads the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAppEnv          = "development"
	defaultHTTPPort        = 8080
	defaultLogLevel        = "info"
	defaultShutdownTimeout = 10 * time.Second
)

// Config holds every setting the service reads at startup.
type Config struct {
	AppEnv      string
	HTTPPort    int
	LogLevel    slog.Level
	DatabaseURL string
	RabbitMQURL string
	// LocalWebhookToken authenticates POST /webhooks/local; empty rejects every request.
	LocalWebhookToken string
	ShutdownTimeout   time.Duration
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

// load reads the configuration using getenv, which makes it testable without
// touching the real environment.
func load(getenv func(string) string) (Config, error) {
	cfg := Config{
		AppEnv:          defaultAppEnv,
		HTTPPort:        defaultHTTPPort,
		LogLevel:        slog.LevelInfo,
		ShutdownTimeout: defaultShutdownTimeout,
	}
	var errs []error

	if v := getenv("APP_ENV"); v != "" {
		cfg.AppEnv = v
	}

	if v := getenv("HTTP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			errs = append(errs, fmt.Errorf("HTTP_PORT: invalid port %q", v))
		} else {
			cfg.HTTPPort = port
		}
	}

	level := getenv("LOG_LEVEL")
	if level == "" {
		level = defaultLogLevel
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: invalid level %q (use debug, info, warn or error)", level))
	}

	cfg.DatabaseURL = getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL: required"))
	}

	cfg.RabbitMQURL = getenv("RABBITMQ_URL")
	if cfg.RabbitMQURL == "" {
		errs = append(errs, errors.New("RABBITMQ_URL: required"))
	}

	cfg.LocalWebhookToken = getenv("LOCAL_WEBHOOK_TOKEN")

	if v := getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: invalid duration %q", v))
		} else {
			cfg.ShutdownTimeout = d
		}
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}
