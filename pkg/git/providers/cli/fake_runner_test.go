package cli

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// call records one runner invocation.
type call struct {
	args []string
	dir  string
	env  []string
}

// response scripts the result for a matched invocation.
type response struct {
	result atmosgit.RunResult
	err    error
}

// fakeRunner records calls and returns scripted responses matched by the
// first non-flag argument sequence (joined args prefix).
type fakeRunner struct {
	calls     []call
	responses map[string][]response
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{responses: map[string][]response{}}
}

// on scripts responses for invocations whose joined args start with prefix.
// Multiple responses for the same prefix are consumed in order; the last one
// repeats.
func (f *fakeRunner) on(prefix string, result atmosgit.RunResult, err error) {
	f.responses[prefix] = append(f.responses[prefix], response{result: result, err: err})
}

// exitErr fabricates the error the real runner returns for a non-zero exit.
func exitErr(code int) error {
	return fmt.Errorf("%w: git (exit %d)", errUtils.ErrGitCommandExited, code)
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
	f.calls = append(f.calls, call{args: args, dir: opts.Dir, env: opts.Env})

	joined := strings.Join(args, " ")
	var bestPrefix string
	for prefix := range f.responses {
		if strings.HasPrefix(joined, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix == "" {
		return atmosgit.RunResult{}, nil
	}

	queue := f.responses[bestPrefix]
	resp := queue[0]
	if len(queue) > 1 {
		f.responses[bestPrefix] = queue[1:]
	}
	return resp.result, resp.err
}

// joinedCalls renders recorded calls for assertions.
func (f *fakeRunner) joinedCalls() []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, strings.Join(c.args, " "))
	}
	return out
}
