package host_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/execution/host"
)

// testImage runs the shell command in $CMD, standing in for the agent image.
const testImage = "miner-host-test:latest"

// dockerRunner returns a runner on the local engine and builds the test
// image, skipping the test when there is no engine.
func dockerRunner(t *testing.T) (*host.Runner, *client.Client) {
	t.Helper()
	cli, err := host.NewClient()
	if err != nil {
		t.Skipf("no Docker client: %v", err)
	}
	if _, err := cli.Ping(context.Background(), client.PingOptions{}); err != nil {
		t.Skipf("Docker engine unavailable: %v", err)
	}
	build := exec.Command("docker", "build", "-q", "-t", testImage, "-")
	build.Stdin = strings.NewReader("FROM alpine:3.20\nENTRYPOINT [\"sh\", \"-c\", \"eval \\\"$CMD\\\"\"]\n")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test image: %v\n%s", err, out)
	}
	return host.New(cli), cli
}

func job(t *testing.T, cmd string) execution.Job {
	t.Helper()
	return execution.Job{RunID: uuid.New(), Dir: t.TempDir(), Image: testImage, Env: map[string]string{"CMD": cmd}}
}

func runContainers(t *testing.T, cli *client.Client) int {
	t.Helper()
	list, err := cli.ContainerList(context.Background(), client.ContainerListOptions{
		All: true, Filters: client.Filters{}.Add("label", host.LabelRun),
	})
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

func TestRun(t *testing.T) {
	r, cli := dockerRunner(t)
	ctx := context.Background()

	missing := job(t, "true")
	missing.Image = "miner-host-test-missing:latest"
	if _, err := r.Run(ctx, missing); !errors.Is(err, host.ErrImageNotFound) || !strings.Contains(err.Error(), "make agent-image") {
		t.Errorf("missing image error = %v", err)
	}

	// The job sees the run directory and its environment, as user 1000.
	j := job(t, `[ "$(id -u)" = 1000 ] && echo ok > /job/out && exit 3`)
	if code, err := r.Run(ctx, j); err != nil || code != 3 {
		t.Errorf("exit code = %d, %v, want 3", code, err)
	}
	if out, err := exec.Command("cat", j.Dir+"/out").Output(); err != nil || string(out) != "ok\n" {
		t.Errorf("run directory not written as uid 1000: %q, %v", out, err)
	}

	// The compose network is out of reach.
	if code, _ := r.Run(ctx, job(t, "nslookup postgres >/dev/null 2>&1")); code == 0 {
		t.Error("job resolved the compose host postgres")
	}

	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := r.Run(cctx, job(t, "sleep 60")); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancelled run error = %v", err)
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("cancellation took %s", took)
	}
	if n := runContainers(t, cli); n != 0 {
		t.Errorf("%d run containers left", n)
	}
}

func TestCleanup(t *testing.T) {
	r, cli := dockerRunner(t)
	ctx := context.Background()

	// A container left behind by a previous worker.
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:  testImage,
			Env:    []string{"CMD=sleep 60"},
			Labels: map[string]string{host.LabelRun: uuid.NewString()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if n := runContainers(t, cli); n != 0 {
		t.Errorf("%d run containers left after cleanup", n)
	}
}
