package claude_test

import (
	"reflect"
	"testing"

	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/execution/claude"
)

func TestModels(t *testing.T) {
	efforts := []string{"low", "medium", "high", "xhigh", "max"}
	want := []execution.Model{
		{ID: "fable", Efforts: efforts},
		{ID: "opus", Efforts: efforts},
		{ID: "sonnet", Efforts: efforts},
	}
	a := claude.New()
	if a.Name() != "claude" {
		t.Errorf("Name() = %q", a.Name())
	}
	if got := a.Models(); !reflect.DeepEqual(got, want) {
		t.Errorf("Models() = %+v, want %+v", got, want)
	}
}

func TestCommand(t *testing.T) {
	a := claude.New()
	tail := []string{"--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	low := "low"
	tests := map[string]struct {
		in   execution.Invocation
		want []string
	}{
		"with effort":    {execution.Invocation{Prompt: "p", Model: "sonnet", Effort: &low}, append([]string{"claude", "-p", "--model", "sonnet", "--effort", "low"}, tail...)},
		"without effort": {execution.Invocation{Prompt: "p", Model: "opus"}, append([]string{"claude", "-p", "--model", "opus"}, tail...)},
	}
	for name, tt := range tests {
		if got := a.Command(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Command() = %v, want %v", name, got, tt.want)
		}
	}
}

func TestCredential(t *testing.T) {
	tests := map[string]struct {
		env    map[string]string
		method string
		ok     bool
	}{
		"oauth only":   {map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "o"}, "oauth", true},
		"api key only": {map[string]string{"ANTHROPIC_API_KEY": "k"}, "api_key", true},
		"both":         {map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "o", "ANTHROPIC_API_KEY": "k"}, "oauth", true},
		"none":         {map[string]string{}, "", false},
	}
	for name, tt := range tests {
		c, ok := claude.New().Credential(func(k string) string { return tt.env[k] })
		if ok != tt.ok || c.Method != tt.method || (ok && c.Value != tt.env[c.Env]) {
			t.Errorf("%s: Credential() = %+v, %v", name, c, ok)
		}
	}
}
