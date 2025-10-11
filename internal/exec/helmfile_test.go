package exec

import (
	"testing"

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
