// Package host is the "host" runner: each job is a disposable container on
// the Docker engine of the machine running the worker.
package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/jrgomesdev/miner/internal/execution"
)

const (
	// Name identifies the runner in the catalog.
	Name = "host"
	// LabelRun marks every container of a run with the run id.
	LabelRun = "miner.run"
	// Network keeps the jobs out of the compose network, so they cannot
	// reach the database or the broker by name.
	Network = "miner-runs"
	// JobDir is where the run directory is mounted inside the container.
	JobDir = "/job"
	// user owns the files written to the run directory.
	user = "1000:1000"
	// stopTimeout bounds the calls made after the job's context ended.
	stopTimeout = 30 * time.Second
)

// ErrImageNotFound means the agent image was not built.
var ErrImageNotFound = errors.New("agent image not found")

// Runner runs jobs as containers.
type Runner struct {
	cli *client.Client
}

// NewClient connects to the Docker engine set by the environment
// (DOCKER_HOST, by default the local socket).
func NewClient() (*client.Client, error) {
	return client.New(client.FromEnv)
}

// New returns the runner that uses cli.
func New(cli *client.Client) *Runner {
	return &Runner{cli: cli}
}

var _ execution.Runner = (*Runner)(nil)

func (r *Runner) Name() string { return Name }

// Run creates the job's container, waits for it to exit and removes it.
// Cancelling ctx kills the container.
func (r *Runner) Run(ctx context.Context, j execution.Job) (int, error) {
	if _, err := r.cli.ImageInspect(ctx, j.Image); err != nil {
		if cerrdefs.IsNotFound(err) {
			return 0, fmt.Errorf("%w: %s (build it with `make agent-image`)", ErrImageNotFound, j.Image)
		}
		return 0, fmt.Errorf("inspect image %s: %w", j.Image, err)
	}
	if err := r.ensureNetwork(ctx); err != nil {
		return 0, err
	}

	env := make([]string, 0, len(j.Env))
	for k, v := range j.Env {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	created, err := r.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: "miner-run-" + j.RunID.String(),
		Config: &container.Config{
			Image:  j.Image,
			Env:    env,
			User:   user,
			Labels: map[string]string{LabelRun: j.RunID.String()},
		},
		HostConfig: &container.HostConfig{
			Mounts:      []mount.Mount{{Type: mount.TypeBind, Source: j.Dir, Target: JobDir}},
			NetworkMode: container.NetworkMode(Network),
			CapDrop:     []string{"ALL"},
			SecurityOpt: []string{"no-new-privileges"},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("create container: %w", err)
	}
	defer r.remove(created.ID)

	waited := r.cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNextExit})
	if _, err := r.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return 0, fmt.Errorf("start container: %w", err)
	}
	select {
	case res := <-waited.Result:
		if res.Error != nil {
			return 0, fmt.Errorf("wait container: %s", res.Error.Message)
		}
		return int(res.StatusCode), nil
	case err := <-waited.Error:
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("wait container: %w", err)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Cleanup removes every container left by a run.
func (r *Runner) Cleanup(ctx context.Context) error {
	list, err := r.cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: client.Filters{}.Add("label", LabelRun),
	})
	if err != nil {
		return fmt.Errorf("list run containers: %w", err)
	}
	var errs []error
	for _, c := range list.Items {
		if _, err := r.cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("remove container %s: %w", c.ID, err))
		}
	}
	return errors.Join(errs...)
}

// remove force-removes a container, which also kills it, even after the
// job's context ended.
func (r *Runner) remove(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	_, _ = r.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
}

func (r *Runner) ensureNetwork(ctx context.Context) error {
	if _, err := r.cli.NetworkInspect(ctx, Network, client.NetworkInspectOptions{}); err == nil {
		return nil
	} else if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspect network %s: %w", Network, err)
	}
	_, err := r.cli.NetworkCreate(ctx, Network, client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{LabelRun: ""},
	})
	if err != nil && !cerrdefs.IsConflict(err) {
		return fmt.Errorf("create network %s: %w", Network, err)
	}
	return nil
}

// RunsDirOnHost returns the host path mounted at containerPath in the
// container this process runs in, so the jobs can mount the same directory.
func RunsDirOnHost(ctx context.Context, cli *client.Client, containerPath string) (string, error) {
	self, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("hostname: %w", err)
	}
	info, err := cli.ContainerInspect(ctx, self, client.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspect own container %s: %w", self, err)
	}
	for _, m := range info.Container.Mounts {
		if m.Destination == containerPath {
			return m.Source, nil
		}
	}
	return "", fmt.Errorf("no mount at %s in container %s", containerPath, self)
}
