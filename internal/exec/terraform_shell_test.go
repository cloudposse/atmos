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

func TestShellInfoFromOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     *ShellOptions
		expected schema.ConfigAndStacksInfo
	}{
		{
			name: "all fields populated",
			opts: &ShellOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
				DryRun:    true,
				Identity:  "dev-role",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
					Skip:             []string{"!terraform.state"},
				},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Stack:            "dev-us-west-2",
				StackFromArg:     "dev-us-west-2",
				ComponentType:    "terraform",
				SubCommand:       "shell",
				DryRun:           true,
				Identity:         "dev-role",
			},
		},
		{
			name: "minimal fields",
			opts: &ShellOptions{
				Component: "rds",
				Stack:     "prod",
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentFromArg: "rds",
				Stack:            "prod",
				StackFromArg:     "prod",
				ComponentType:    "terraform",
				SubCommand:       "shell",
			},
		},
		{
			name: "empty options",
			opts: &ShellOptions{},
			expected: schema.ConfigAndStacksInfo{
				ComponentType: "terraform",
				SubCommand:    "shell",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := shellInfoFromOptions(tt.opts)
			assert.Equal(t, tt.expected.ComponentFromArg, info.ComponentFromArg)
			assert.Equal(t, tt.expected.Stack, info.Stack)
			assert.Equal(t, tt.expected.StackFromArg, info.StackFromArg)
			assert.Equal(t, tt.expected.ComponentType, info.ComponentType)
			assert.Equal(t, tt.expected.SubCommand, info.SubCommand)
			assert.Equal(t, tt.expected.DryRun, info.DryRun)
			assert.Equal(t, tt.expected.Identity, info.Identity)
		})
	}
}

func TestShellInfoFromOptions_StackFromArgMatchesStack(t *testing.T) {
	// Verify StackFromArg is always set to match Stack.
	opts := &ShellOptions{
		Component: "vpc",
		Stack:     "prod-us-east-1",
	}
	info := shellInfoFromOptions(opts)
	assert.Equal(t, info.Stack, info.StackFromArg,
		"StackFromArg must equal Stack for shell commands")
}

func TestResolveWorkdirPath(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		componentPath    string
		expected         string
	}{
		{
			name:             "no workdir key - returns original",
			componentSection: map[string]any{},
			componentPath:    "/components/terraform/vpc",
			expected:         "/components/terraform/vpc",
		},
		{
			name: "workdir set - returns workdir",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: "/workdir/terraform/vpc",
			},
			componentPath: "/components/terraform/vpc",
			expected:      "/workdir/terraform/vpc",
		},
		{
			name: "workdir empty string - returns original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: "",
			},
			componentPath: "/components/terraform/vpc",
			expected:      "/components/terraform/vpc",
		},
		{
			name: "workdir nil - returns original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: nil,
			},
			componentPath: "/components/terraform/vpc",
			expected:      "/components/terraform/vpc",
		},
		{
			name: "workdir wrong type (int) - returns original",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: 123,
			},
			componentPath: "/components/terraform/vpc",
			expected:      "/components/terraform/vpc",
		},
		{
			name: "workdir with other fields present",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: "/workdir/terraform/s3-bucket",
				"component":                "s3-bucket",
				"vars":                     map[string]any{"name": "my-bucket"},
			},
			componentPath: "/components/terraform/s3-bucket",
			expected:      "/workdir/terraform/s3-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkdirPath(tt.componentSection, tt.componentPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveWorkdirPath_NilSection(t *testing.T) {
	// A nil componentSection should not panic.
	result := resolveWorkdirPath(nil, "/components/terraform/vpc")
	assert.Equal(t, "/components/terraform/vpc", result)
}

func TestShellOptionsForUI(t *testing.T) {
	tests := []struct {
		name      string
		component string
		stack     string
	}{
		{
			name:      "typical values",
			component: "vpc",
			stack:     "dev-us-west-2",
		},
		{
			name:      "empty values",
			component: "",
			stack:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := shellOptionsForUI(tt.component, tt.stack)
			assert.Equal(t, tt.component, opts.Component)
			assert.Equal(t, tt.stack, opts.Stack)
			assert.True(t, opts.ProcessTemplates, "UI path must enable template processing")
			assert.True(t, opts.ProcessFunctions, "UI path must enable function processing")
			assert.False(t, opts.DryRun, "UI path does not support dry-run")
			assert.Empty(t, opts.Identity, "UI path does not support identity selection")
			assert.Empty(t, opts.Skip, "UI path does not support skip")
		})
	}
}

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
				_ = w.Close()
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
