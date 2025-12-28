package exec

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestPrintShellDryRunInfo(t *testing.T) {
	tests := []struct {
		name           string
		info           *schema.ConfigAndStacksInfo
		cfg            *shellConfig
		expectedOutput []string
	}{
		{
			name: "Basic configuration",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg:   "vpc",
				Stack:              "dev-us-west-2",
				TerraformWorkspace: "dev-vpc",
			},
			cfg: &shellConfig{
				componentPath: "/terraform/components/vpc",
				workingDir:    "/terraform/components/vpc",
				varFile:       "dev-us-west-2-vpc.terraform.tfvars.json",
			},
			expectedOutput: []string{
				"Dry run mode: shell would be started with the following configuration:",
				"Component: vpc",
				"Stack: dev-us-west-2",
				"Working directory: /terraform/components/vpc",
				"Terraform workspace: dev-vpc",
				"Component path: /terraform/components/vpc",
				"Varfile: dev-us-west-2-vpc.terraform.tfvars.json",
			},
		},
		{
			name: "Empty workspace",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg:   "rds",
				Stack:              "prod",
				TerraformWorkspace: "",
			},
			cfg: &shellConfig{
				componentPath: "/components/terraform/rds",
				workingDir:    "/components/terraform/rds",
				varFile:       "prod-rds.terraform.tfvars.json",
			},
			expectedOutput: []string{
				"Component: rds",
				"Stack: prod",
				"Terraform workspace: ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture UI output (stderr).
			var buf bytes.Buffer

			// Capture stderr where UI output goes.
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			require.NoError(t, err, "failed to create pipe for stderr capture")

			// Ensure stderr is restored and pipe ends are closed even on panic.
			defer func() {
				os.Stderr = oldStderr
				r.Close()
			}()

			os.Stderr = w

			// Initialize the UI formatter with a standard I/O context.
			ioCtx, err := iolib.NewContext()
			require.NoError(t, err, "failed to create I/O context")
			ui.InitFormatter(ioCtx)

			printShellDryRunInfo(tt.info, tt.cfg)

			// Close write end and read the output.
			w.Close()
			_, _ = buf.ReadFrom(r)

			output := buf.String()
			for _, expected := range tt.expectedOutput {
				require.Contains(t, output, expected)
			}
		})
	}
}

// TestWorkdirPathKeyExtraction tests the workdir path key extraction logic
// used in both terraform_shell.go and terraform.go to determine if a workdir provisioner
// has set a custom component path.
func TestWorkdirPathKeyExtraction(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		originalPath     string
		expectedPath     string
		shouldOverride   bool
	}{
		{
			name:             "no workdir path set - use original",
			componentSection: map[string]any{},
			originalPath:     "/components/terraform/vpc",
			expectedPath:     "/components/terraform/vpc",
			shouldOverride:   false,
		},
		{
			name: "workdir path set - override original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: "/workdir/terraform/vpc",
			},
			originalPath:   "/components/terraform/vpc",
			expectedPath:   "/workdir/terraform/vpc",
			shouldOverride: true,
		},
		{
			name: "workdir path empty string - use original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: "",
			},
			originalPath:   "/components/terraform/vpc",
			expectedPath:   "/components/terraform/vpc",
			shouldOverride: false,
		},
		{
			name: "workdir path nil - use original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: nil,
			},
			originalPath:   "/components/terraform/vpc",
			expectedPath:   "/components/terraform/vpc",
			shouldOverride: false,
		},
		{
			name: "workdir path wrong type (int) - use original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: 123,
			},
			originalPath:   "/components/terraform/vpc",
			expectedPath:   "/components/terraform/vpc",
			shouldOverride: false,
		},
		{
			name: "workdir path set with other fields - override original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: "/workdir/terraform/s3-bucket",
				"component":                "s3-bucket",
				"vars": map[string]any{
					"name": "my-bucket",
				},
			},
			originalPath:   "/components/terraform/s3-bucket",
			expectedPath:   "/workdir/terraform/s3-bucket",
			shouldOverride: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentPath := tt.originalPath

			// Simulate the workdir path extraction logic from terraform_shell.go:74-77 and terraform.go:409-411.
			if workdirPath, ok := tt.componentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
				componentPath = workdirPath
			}

			assert.Equal(t, tt.expectedPath, componentPath)

			// Verify whether override happened.
			if tt.shouldOverride {
				assert.NotEqual(t, tt.originalPath, componentPath, "componentPath should have been overridden")
			} else if tt.originalPath != tt.expectedPath {
				// Special case where original and expected differ but no override (shouldn't happen in our tests).
				t.Errorf("test configuration error: originalPath != expectedPath but shouldOverride is false")
			}
		})
	}
}

// TestShellConfigConstruction tests the shellConfig struct construction.
func TestShellConfigConstruction(t *testing.T) {
	cfg := &shellConfig{
		componentPath: "/components/terraform/vpc",
		workingDir:    "/project/components/terraform/vpc",
		varFile:       "dev-vpc.terraform.tfvars.json",
	}

	assert.Equal(t, "/components/terraform/vpc", cfg.componentPath)
	assert.Equal(t, "/project/components/terraform/vpc", cfg.workingDir)
	assert.Equal(t, "dev-vpc.terraform.tfvars.json", cfg.varFile)
}
