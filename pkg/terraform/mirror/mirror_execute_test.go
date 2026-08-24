package mirror

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteMirrorModel_EmptyTargetsIsNoop(t *testing.T) {
	require.NoError(t, executeMirrorModel(nil, nil, nil, 1))
}

// TestRunMirror_HumanFormatDrivesTUI exercises the human-format branch of runMirror,
// which runs executeMirrorModel. In the headless test environment there is no TTY, so
// the model runs without a renderer and the goroutine mirrors each target through the
// executeTerraform seam.
func TestRunMirror_HumanFormatDrivesTUI(t *testing.T) {
	orig := executeTerraform
	t.Cleanup(func() { executeTerraform = orig })

	var (
		mu       sync.Mutex
		gotComps []string
	)
	executeTerraform = func(info schema.ConfigAndStacksInfo, _ ...e.ShellCommandOption) error {
		mu.Lock()
		gotComps = append(gotComps, info.ComponentFromArg)
		mu.Unlock()
		return nil
	}

	targets := []Target{
		{Component: "vpc", Stack: "prod"},
		{Component: "rds", Stack: "prod"},
	}
	require.NoError(t, runMirror("", targets, []string{"-platform=linux_amd64", "out"}, nil, 2))

	mu.Lock()
	defer mu.Unlock()
	assert.ElementsMatch(t, []string{"vpc", "rds"}, gotComps)
}

// TestExecuteMirrorModel_AggregatesFailures verifies that a failing component surfaces
// as ErrTerraformExecFailed listing the failed component(s).
func TestExecuteMirrorModel_AggregatesFailures(t *testing.T) {
	orig := executeTerraform
	t.Cleanup(func() { executeTerraform = orig })
	executeTerraform = func(schema.ConfigAndStacksInfo, ...e.ShellCommandOption) error {
		return errors.New("boom")
	}

	err := executeMirrorModel([]Target{{Component: "vpc", Stack: "prod"}}, nil, nil, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTerraformExecFailed)
}
