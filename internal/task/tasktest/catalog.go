package tasktest

import (
	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/execution/claude"
)

// FakeAgent is the name of the second agent in Catalog. Its model "small"
// takes no effort and "large" takes "low" or "high"; neither is a claude model.
const FakeAgent = "fake"

type fakeAgent struct{}

func (fakeAgent) Name() string { return FakeAgent }

func (fakeAgent) Models() []execution.Model {
	return []execution.Model{{ID: "small"}, {ID: "large", Efforts: []string{"low", "high"}}}
}

func (fakeAgent) Command(in execution.Invocation) []string {
	return []string{"fake-agent", "--model", in.Model}
}

func (fakeAgent) Credential(getenv func(string) string) (execution.Credential, bool) {
	if v := getenv("FAKE_AGENT_TOKEN"); v != "" {
		return execution.Credential{Method: "token", Env: "FAKE_AGENT_TOKEN", Value: v}, true
	}
	return execution.Credential{}, false
}

// Catalog returns the real claude agent, a fake agent and the "host" runner.
func Catalog() *execution.Catalog {
	return execution.NewCatalog([]execution.Agent{claude.New(), fakeAgent{}}, []string{"host"})
}
