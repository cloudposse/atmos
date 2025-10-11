package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestExecuteHelmfile_Version(t *testing.T) {
	tests.RequireHelmfile(t)

	testCases := []struct {
		name           string
		workDir        string
		expectedOutput string
	}{
		{
			name:           "helmfile version",
			workDir:        "../../tests/fixtures/scenarios/atmos-helmfile-version",
			expectedOutput: "helmfile",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Set info for ExecuteHelmfile.
			info := schema.ConfigAndStacksInfo{
				SubCommand: "version",
			}

			testCaptureCommandOutput(t, tt.workDir, func() error {
				return ExecuteHelmfile(info)
			}, tt.expectedOutput)
		})
	}
}

func TestExecuteHelmfile_MissingStack(t *testing.T) {
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	defer func() {
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to starting directory: %v", err)
		}
	}()

	workDir := "../../tests/fixtures/scenarios/complete"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "echo-server",
		Stack:            "",
		SubCommand:       "diff",
	}

	err = ExecuteHelmfile(info)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingStack)
}

func TestExecuteHelmfile_ComponentNotFound(t *testing.T) {
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	defer func() {
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to starting directory: %v", err)
		}
	}()

	workDir := "../../tests/fixtures/scenarios/complete"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "non-existent-component",
		Stack:            "tenant1-ue2-dev",
		SubCommand:       "diff",
	}

	err = ExecuteHelmfile(info)
	assert.Error(t, err)
	// ExecuteHelmfile calls ProcessStacks which will fail to find the component.
	assert.Contains(t, err.Error(), "Could not find the component")
}
