// Package execution describes what can run a task: the AI agents with their
// models and effort levels, and the runners available. It does not depend on
// the task domain.
package execution

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// Agent is an AI agent able to work on a task.
type Agent interface {
	// Name identifies the agent in the catalog and in task configuration.
	Name() string
	// Models lists the models the agent accepts, in display order.
	Models() []Model
	// Command returns the argv that runs the agent without interaction. The
	// prompt is written to its standard input, and the agent writes its
	// events to standard output as JSON lines.
	Command(Invocation) []string
	// Credential picks, in the agent's order of priority, the credential
	// found through getenv. It reports false when none is configured.
	Credential(getenv func(string) string) (Credential, bool)
}

// Invocation is what an agent receives to work on a task.
type Invocation struct {
	Prompt string
	Model  string
	// Effort is nil when the model takes no effort level.
	Effort *string
}

// Credential authenticates an agent inside a run environment.
type Credential struct {
	// Method is recorded on the run, for example "oauth" or "api_key".
	Method string
	// Env is the variable the agent reads Value from.
	Env   string
	Value string
}

// Job is a run handed to a runner.
type Job struct {
	RunID uuid.UUID
	// Dir is the run directory on the host, mounted into the environment.
	Dir   string
	Image string
	Env   map[string]string
}

// Runner executes jobs in an isolated, disposable environment.
type Runner interface {
	// Name matches a runner name of the catalog.
	Name() string
	// Run executes j until it exits and returns its exit code. Cancelling
	// ctx stops the environment and returns ctx's error.
	Run(ctx context.Context, j Job) (int, error)
	// Cleanup removes the environments left behind by a previous process.
	Cleanup(ctx context.Context) error
}

// Model is a model an agent accepts.
type Model struct {
	ID string
	// Efforts lists the effort levels in display order; empty means the model
	// takes no effort level.
	Efforts []string
}

// FindModel returns the model of a with id.
func FindModel(a Agent, id string) (Model, bool) {
	i := slices.IndexFunc(a.Models(), func(m Model) bool { return m.ID == id })
	if i < 0 {
		return Model{}, false
	}
	return a.Models()[i], true
}

// Catalog holds the registered agents and the available runners. It is
// fixed once built.
type Catalog struct {
	agents  map[string]Agent
	names   []string
	runners []string
}

// NewCatalog indexes agents by name. A duplicate agent name is a programming
// error and panics.
func NewCatalog(agents []Agent, runners []string) *Catalog {
	c := &Catalog{agents: make(map[string]Agent, len(agents))}
	for _, a := range agents {
		if _, dup := c.agents[a.Name()]; dup {
			panic(fmt.Sprintf("execution: agent %q registered twice", a.Name()))
		}
		c.agents[a.Name()] = a
		c.names = append(c.names, a.Name())
	}
	slices.Sort(c.names)
	c.runners = slices.Compact(slices.Sorted(slices.Values(runners)))
	return c
}

// Agent returns the agent registered with name.
func (c *Catalog) Agent(name string) (Agent, bool) {
	a, ok := c.agents[name]
	return a, ok
}

// HasRunner reports whether name is an available runner.
func (c *Catalog) HasRunner(name string) bool {
	_, ok := slices.BinarySearch(c.runners, name)
	return ok
}

// Options is the catalog in display order: agents and runners by name,
// models and efforts in the order defined by each agent.
type Options struct {
	Agents  []AgentOptions
	Runners []string
}

// AgentOptions is an agent and its models.
type AgentOptions struct {
	Name   string
	Models []Model
}

// Options returns a copy of the catalog in display order.
func (c *Catalog) Options() Options {
	opts := Options{Agents: make([]AgentOptions, 0, len(c.names)), Runners: slices.Clone(c.runners)}
	for _, name := range c.names {
		models := make([]Model, 0, len(c.agents[name].Models()))
		for _, m := range c.agents[name].Models() {
			models = append(models, Model{ID: m.ID, Efforts: slices.Clone(m.Efforts)})
		}
		opts.Agents = append(opts.Agents, AgentOptions{Name: name, Models: models})
	}
	return opts
}
