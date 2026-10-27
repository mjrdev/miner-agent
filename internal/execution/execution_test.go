package execution_test

import (
	"reflect"
	"testing"

	"github.com/jrgomesdev/miner/internal/execution"
)

type fakeAgent struct {
	name   string
	models []execution.Model
}

func (a fakeAgent) Name() string              { return a.name }
func (a fakeAgent) Models() []execution.Model { return a.models }

func (fakeAgent) Command(execution.Invocation) []string { return nil }

func (fakeAgent) Credential(func(string) string) (execution.Credential, bool) {
	return execution.Credential{}, false
}

func TestCatalog(t *testing.T) {
	zeta := fakeAgent{name: "zeta", models: []execution.Model{{ID: "b"}, {ID: "a", Efforts: []string{"high", "low"}}}}
	alpha := fakeAgent{name: "alpha", models: []execution.Model{{ID: "m"}}}
	c := execution.NewCatalog([]execution.Agent{zeta, alpha}, []string{"host"})

	if a, ok := c.Agent("zeta"); !ok || a.Name() != "zeta" {
		t.Errorf("Agent(zeta) = %v, %v", a, ok)
	}
	if _, ok := c.Agent("gpt"); ok {
		t.Error("Agent(gpt) found")
	}
	if !c.HasRunner("host") {
		t.Error(`HasRunner("host") = false`)
	}
	if c.HasRunner("k8s") {
		t.Error(`HasRunner("k8s") = true`)
	}

	want := execution.Options{
		Agents: []execution.AgentOptions{
			{Name: "alpha", Models: []execution.Model{{ID: "m"}}},
			{Name: "zeta", Models: []execution.Model{{ID: "b"}, {ID: "a", Efforts: []string{"high", "low"}}}},
		},
		Runners: []string{"host"},
	}
	if got := c.Options(); !reflect.DeepEqual(got, want) {
		t.Errorf("Options() = %+v, want %+v", got, want)
	}

	if m, ok := execution.FindModel(zeta, "a"); !ok || len(m.Efforts) != 2 {
		t.Errorf("FindModel(zeta, a) = %+v, %v", m, ok)
	}
	if _, ok := execution.FindModel(zeta, "x"); ok {
		t.Error("FindModel(zeta, x) found")
	}
}
