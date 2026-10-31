package starlark

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/script"
)

// scheduledFake is a step library that records the scheduled index of each fork.
type scheduledFake struct {
	forks *[]int
}

func (scheduledFake) Names() []string                                             { return nil }
func (scheduledFake) Validate(*api.StepCall) error                                { return nil }
func (scheduledFake) Run(context.Context, *api.StepCall) (*api.StepResult, error) { return nil, nil }

func (f scheduledFake) Fork() api.StepLibrary { *f.forks = append(*f.forks, -1); return f }

func (f scheduledFake) ForkAt(index int) api.StepLibrary {
	*f.forks = append(*f.forks, index)
	return f
}

func TestScheduleBranchForksEveryAttemptAtTheBranchIndex(t *testing.T) {
	var forks []int
	parent := scheduledFake{forks: &forks}
	branch := scheduleBranch(parent, 3)

	// Each retry attempt forks again; every fork records the same scheduled index.
	forkSteps(branch)
	forkSteps(branch)
	assert.Equal(t, []int{3, 3}, forks)
}

func TestScheduleBranchLeavesOtherLibrariesAlone(t *testing.T) {
	assert.Nil(t, scheduleBranch(nil, 1))

	var forks []int
	// A library that does not implement api.ScheduledForker keeps forking normally.
	var plain api.StepLibrary = struct{ api.StepLibrary }{scheduledFake{forks: &forks}}
	assert.Equal(t, plain, scheduleBranch(plain, 1))
	forkSteps(scheduleBranch(plain, 1))
	assert.Equal(t, []int{-1}, forks)
}

// TestParallelForksEachBranchAtItsScheduledIndex runs a real script and checks that the library
// the engine hands to a branch was forked at the branch's index in the group.
func TestParallelForksEachBranchAtItsScheduledIndex(t *testing.T) {
	var (
		mu    sync.Mutex
		forks []int
	)
	library := &lockedFake{mu: &mu, forks: &forks}
	_, err := New().Execute(t.Context(), script.Spec{
		Steps:  library,
		Source: "def work(name):\n    return name\nsteps.parallel(tasks = [steps.task(name = n, function = work, args = [n]) for n in [\"a\", \"b\", \"c\"]], max_concurrency = 3)\n",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.ElementsMatch(t, []int{0, 1, 2}, scheduledIndexes(forks), "every branch forks at its own scheduled index")
}

// scheduledIndexes drops the unscheduled forks (the engine's own thread fork).
func scheduledIndexes(forks []int) []int {
	var scheduled []int
	for _, index := range forks {
		if index >= 0 {
			scheduled = append(scheduled, index)
		}
	}
	return scheduled
}

// lockedFake is a concurrency-safe scheduled step library.
type lockedFake struct {
	mu    *sync.Mutex
	forks *[]int
}

func (*lockedFake) Names() []string                                             { return nil }
func (*lockedFake) Validate(*api.StepCall) error                                { return nil }
func (*lockedFake) Run(context.Context, *api.StepCall) (*api.StepResult, error) { return nil, nil }

func (l *lockedFake) Fork() api.StepLibrary {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.forks = append(*l.forks, -1)
	return l
}

func (l *lockedFake) ForkAt(index int) api.StepLibrary {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.forks = append(*l.forks, index)
	return l
}
