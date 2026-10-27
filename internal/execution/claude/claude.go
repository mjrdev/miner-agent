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

// Command runs the CLI in print mode, reading the prompt from standard input
// and streaming its events as JSON lines. Tools run without asking, so the
// run environment is the security boundary.
func (Agent) Command(in execution.Invocation) []string {
	argv := []string{"claude", "-p", "--model", in.Model}
	if in.Effort != nil {
		argv = append(argv, "--effort", *in.Effort)
	}
	return append(argv, "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions")
}

// credentials are the variables the CLI reads, in order of priority: the
// subscription token comes before the pay-per-use API key.
var credentials = []struct{ method, env string }{
	{"oauth", "CLAUDE_CODE_OAUTH_TOKEN"},
	{"api_key", "ANTHROPIC_API_KEY"},
}

func (Agent) Credential(getenv func(string) string) (execution.Credential, bool) {
	for _, c := range credentials {
		if v := getenv(c.env); v != "" {
			return execution.Credential{Method: c.method, Env: c.env, Value: v}, true
		}
	}
	return execution.Credential{}, false
}
