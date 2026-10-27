package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadWorker(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://db", "RABBITMQ_URL": "amqp://mq"}
	with := func(extra map[string]string) func(string) string {
		return func(k string) string {
			if v, ok := extra[k]; ok {
				return v
			}
			return base[k]
		}
	}

	got, err := loadWorker(with(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := Worker{
		LogLevel: slog.LevelInfo, DatabaseURL: "postgres://db", RabbitMQURL: "amqp://mq",
		ShutdownTimeout: 10 * time.Second, MaxConcurrentRuns: 1, RunTimeout: time.Hour,
		AgentImage: "miner-agent:default", RunsDir: "/runs",
	}
	if got != want {
		t.Errorf("defaults = %+v, want %+v", got, want)
	}

	got, err = loadWorker(with(map[string]string{
		"MAX_CONCURRENT_RUNS": "3", "RUN_TIMEOUT": "90s", "AGENT_IMAGE": "img:1",
		"RUNS_DIR": "/data/runs", "RUNS_HOST_DIR": "/home/me/miner/runs", "GIT_TOKEN": "t",
	}))
	if err != nil || got.MaxConcurrentRuns != 3 || got.RunTimeout != 90*time.Second || got.AgentImage != "img:1" ||
		got.RunsDir != "/data/runs" || got.RunsHostDir != "/home/me/miner/runs" || got.GitToken != "t" {
		t.Errorf("custom values = %+v, %v", got, err)
	}

	for name, env := range map[string]map[string]string{
		"MAX_CONCURRENT_RUNS": {"MAX_CONCURRENT_RUNS": "0"},
		"RUN_TIMEOUT":         {"RUN_TIMEOUT": "forever"},
		"DATABASE_URL":        {"DATABASE_URL": ""},
	} {
		if _, err := loadWorker(with(env)); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}
