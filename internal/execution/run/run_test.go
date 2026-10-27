package run

import (
	"testing"

	"github.com/jrgomesdev/miner/internal/task"
)

func TestTransitions(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusQueued, StatusRunning}:    true,
		{StatusQueued, StatusFailed}:     true,
		{StatusQueued, StatusCancelled}:  true,
		{StatusRunning, StatusSucceeded}: true,
		{StatusRunning, StatusFailed}:    true,
		{StatusRunning, StatusCancelled}: true,
	}
	all := []Status{StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusCancelled}
	for _, from := range all {
		for _, to := range all {
			if got, want := from.CanTransitionTo(to), allowed[[2]Status{from, to}]; got != want {
				t.Errorf("%s -> %s = %v, want %v", from, to, got, want)
			}
		}
		if got, want := from.Active(), from == StatusQueued || from == StatusRunning; got != want {
			t.Errorf("%s.Active() = %v", from, got)
		}
	}
}

func TestTaskStatusAfterIsAValidTransition(t *testing.T) {
	want := map[Status]task.Status{
		StatusSucceeded: task.StatusInReview,
		StatusFailed:    task.StatusFailed,
		StatusCancelled: task.StatusReady,
	}
	for s, w := range want {
		got := TaskStatusAfter(s)
		if got != w || !task.StatusInProgress.CanTransitionTo(got) {
			t.Errorf("TaskStatusAfter(%s) = %s, want %s reachable from in_progress", s, got, w)
		}
	}
}
