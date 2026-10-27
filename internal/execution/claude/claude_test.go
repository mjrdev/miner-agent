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
