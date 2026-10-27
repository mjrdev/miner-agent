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
	defaultMaxConcurrentRuns = 1
	defaultRunTimeout        = 60 * time.Minute
	defaultAgentImage        = "miner-agent:default"
	defaultRunsDir           = "/runs"
)

// Worker holds the settings of the run worker. The agent credentials are not
// here: each agent reads its own variables.
type Worker struct {
	LogLevel        slog.Level
	DatabaseURL     string
	RabbitMQURL     string
	ShutdownTimeout time.Duration
	// MaxConcurrentRuns is how many runs execute at the same time.
	MaxConcurrentRuns int
	// RunTimeout ends a run that takes longer.
	RunTimeout time.Duration
	// AgentImage is the image every run executes in.
	AgentImage string
	// RunsDir is where the worker writes the run directories.
	RunsDir string
	// RunsHostDir is RunsDir as the Docker engine sees it; empty means it is
	// discovered from the worker's own container.
	RunsHostDir string
	// GitToken, when set, clones private repositories.
	GitToken string
}

// LoadWorker reads the worker configuration from the process environment.
func LoadWorker() (Worker, error) {
	return loadWorker(os.Getenv)
}

func loadWorker(getenv func(string) string) (Worker, error) {
	cfg := Worker{
		LogLevel:          slog.LevelInfo,
		ShutdownTimeout:   defaultShutdownTimeout,
		MaxConcurrentRuns: defaultMaxConcurrentRuns,
		RunTimeout:        defaultRunTimeout,
		AgentImage:        defaultAgentImage,
		RunsDir:           defaultRunsDir,
	}
	var errs []error

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

	if v := getenv("SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: invalid duration %q", v))
		} else {
			cfg.ShutdownTimeout = d
		}
	}
	if v := getenv("RUN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("RUN_TIMEOUT: invalid duration %q", v))
		} else {
			cfg.RunTimeout = d
		}
	}
	if v := getenv("MAX_CONCURRENT_RUNS"); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			errs = append(errs, fmt.Errorf("MAX_CONCURRENT_RUNS: must be a positive integer, got %q", v))
		} else {
			cfg.MaxConcurrentRuns = n
		}
	}
	if v := getenv("AGENT_IMAGE"); v != "" {
		cfg.AgentImage = v
	}
	if v := getenv("RUNS_DIR"); v != "" {
		cfg.RunsDir = v
	}
	cfg.RunsHostDir = getenv("RUNS_HOST_DIR")
	cfg.GitToken = getenv("GIT_TOKEN")

	if len(errs) > 0 {
		return Worker{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}
