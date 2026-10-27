// Package claude is the execution agent for the Claude Code CLI.
package claude

import "github.com/jrgomesdev/miner/internal/execution"

// Name identifies the agent.
const Name = "claude"

// efforts are the levels accepted by `claude --effort`.
var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// models are the CLI aliases, which always point to the latest version.
var models = []string{"fable", "opus", "sonnet"}

// Agent is the Claude Code agent.
type Agent struct{}

// New returns the Claude Code agent.
func New() Agent { return Agent{} }

var _ execution.Agent = Agent{}

func (Agent) Name() string { return Name }

func (Agent) Models() []execution.Model {
	out := make([]execution.Model, 0, len(models))
	for _, id := range models {
		out = append(out, execution.Model{ID: id, Efforts: append([]string{}, efforts...)})
	}
	return out
}
