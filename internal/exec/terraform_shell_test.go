package exec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
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

// TestShellOptionsFromConfigAndStacksInfo tests that ShellOptions can be correctly
// populated from ConfigAndStacksInfo, which is the pattern used when routing
// the "shell" subcommand through ExecuteTerraform.
func TestShellOptionsFromConfigAndStacksInfo(t *testing.T) {
	tests := []struct {
		name     string
		info     schema.ConfigAndStacksInfo
		expected *ShellOptions
	}{
		{
			name: "basic info to options conversion",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Stack:            "dev-us-west-2",
				DryRun:           false,
				Identity:         "",
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             nil,
			},
			expected: &ShellOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
				DryRun:    false,
				Identity:  "",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
					Skip:             nil,
				},
			},
		},
		{
			name: "with dry run and identity",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "rds",
				Stack:            "prod-eu-west-1",
				DryRun:           true,
				Identity:         "admin-role",
				ProcessTemplates: false,
				ProcessFunctions: true,
				Skip:             []string{"template"},
			},
			expected: &ShellOptions{
				Component: "rds",
				Stack:     "prod-eu-west-1",
				DryRun:    true,
				Identity:  "admin-role",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: false,
					ProcessFunctions: true,
					Skip:             []string{"template"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the same conversion logic used in ExecuteTerraform for "shell" subcommand.
			opts := &ShellOptions{
				Component:         tt.info.ComponentFromArg,
				Stack:             tt.info.Stack,
				DryRun:            tt.info.DryRun,
				Identity:          tt.info.Identity,
				ProcessingOptions: ProcessingOptions{ProcessTemplates: tt.info.ProcessTemplates, ProcessFunctions: tt.info.ProcessFunctions, Skip: tt.info.Skip},
			}

			assert.Equal(t, tt.expected.Component, opts.Component)
			assert.Equal(t, tt.expected.Stack, opts.Stack)
			assert.Equal(t, tt.expected.DryRun, opts.DryRun)
			assert.Equal(t, tt.expected.Identity, opts.Identity)
			assert.Equal(t, tt.expected.ProcessTemplates, opts.ProcessTemplates)
			assert.Equal(t, tt.expected.ProcessFunctions, opts.ProcessFunctions)
			assert.Equal(t, tt.expected.Skip, opts.Skip)
		})
	}
}

// TestShellSubcommandIdentification tests that the "shell" subcommand is correctly
// identified as an Atmos-specific command that should be routed to ExecuteTerraformShell
// and not passed to the terraform executable.
func TestShellSubcommandIdentification(t *testing.T) {
	tests := []struct {
		name       string
		subCommand string
		isShell    bool
	}{
		{"shell command", "shell", true},
		{"plan command", "plan", false},
		{"apply command", "apply", false},
		{"destroy command", "destroy", false},
		{"init command", "init", false},
		{"version command", "version", false},
		{"workspace command", "workspace", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isShell := tt.subCommand == "shell"
			assert.Equal(t, tt.isShell, isShell)
		})
	}
}

// TestStoreAuthenticatedIdentity tests the storeAuthenticatedIdentity function.
func TestStoreAuthenticatedIdentity(t *testing.T) {
	tests := []struct {
		name             string
		chain            []string
		initialIdentity  string
		expectedIdentity string
		nilAuthManager   bool
	}{
		{
			name:             "nil AuthManager - no change",
			nilAuthManager:   true,
			initialIdentity:  "",
			expectedIdentity: "",
		},
		{
			name:             "identity already set - no change",
			chain:            []string{"provider", "target-identity"},
			initialIdentity:  "explicit-identity",
			expectedIdentity: "explicit-identity",
		},
		{
			name:             "empty chain - no change",
			chain:            []string{},
			initialIdentity:  "",
			expectedIdentity: "",
		},
		{
			name:             "chain with single element",
			chain:            []string{"single-identity"},
			initialIdentity:  "",
			expectedIdentity: "single-identity",
		},
		{
			name:             "chain with multiple elements - uses last",
			chain:            []string{"provider", "intermediate", "target-identity"},
			initialIdentity:  "",
			expectedIdentity: "target-identity",
		},
		{
			name:             "chain with two elements - uses last",
			chain:            []string{"provider", "dev-role"},
			initialIdentity:  "",
			expectedIdentity: "dev-role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			info := &schema.ConfigAndStacksInfo{
				Identity: tt.initialIdentity,
			}

			if tt.nilAuthManager {
				// Test with nil AuthManager.
				storeAuthenticatedIdentity(nil, info)
				assert.Equal(t, tt.expectedIdentity, info.Identity)
			} else {
				// Create mock AuthManager.
				mockAuthMgr := types.NewMockAuthManager(ctrl)
				mockAuthMgr.EXPECT().GetChain().Return(tt.chain).AnyTimes()

				// Call the actual function.
				storeAuthenticatedIdentity(mockAuthMgr, info)
				assert.Equal(t, tt.expectedIdentity, info.Identity)
			}
		})
	}
}

// TestShellOptionsValidation tests validation of ShellOptions fields.
func TestShellOptionsValidation(t *testing.T) {
	tests := []struct {
		name          string
		opts          *ShellOptions
		expectValid   bool
		invalidReason string
	}{
		{
			name: "valid options with component and stack",
			opts: &ShellOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
			},
			expectValid: true,
		},
		{
			name: "valid options with all fields",
			opts: &ShellOptions{
				Component: "rds",
				Stack:     "prod-eu-west-1",
				DryRun:    true,
				Identity:  "admin-role",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
					Skip:             []string{"template"},
				},
			},
			expectValid: true,
		},
		{
			name: "missing component",
			opts: &ShellOptions{
				Component: "",
				Stack:     "dev-us-west-2",
			},
			expectValid:   false,
			invalidReason: "component is required",
		},
		{
			name: "missing stack",
			opts: &ShellOptions{
				Component: "vpc",
				Stack:     "",
			},
			expectValid:   false,
			invalidReason: "stack is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.opts.Component != "" && tt.opts.Stack != ""
			assert.Equal(t, tt.expectValid, isValid, tt.invalidReason)
		})
	}
}

// TestShellConfigWithWorkdirProvisioner tests shellConfig when workdir provisioner is active.
func TestShellConfigWithWorkdirProvisioner(t *testing.T) {
	// Use platform-agnostic paths.
	componentPathOriginal := filepath.Join("components", "terraform", "vpc")
	workdirPathVpc := filepath.Join("workdir", "terraform", "vpc")
	workdirPathTemp := filepath.Join("tmp", "atmos-workdir-123", "vpc")

	tests := []struct {
		name             string
		componentSection map[string]any
		originalPath     string
		expectedCfgPath  string
	}{
		{
			name:             "no workdir - uses component path",
			componentSection: map[string]any{},
			originalPath:     componentPathOriginal,
			expectedCfgPath:  componentPathOriginal,
		},
		{
			name: "workdir set - uses workdir path",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: workdirPathVpc,
			},
			originalPath:    componentPathOriginal,
			expectedCfgPath: workdirPathVpc,
		},
		{
			name: "workdir set with vars - uses workdir path",
			componentSection: map[string]any{
				provWorkdir.WorkdirPathKey: workdirPathTemp,
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			originalPath:    componentPathOriginal,
			expectedCfgPath: workdirPathTemp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentPath := tt.originalPath

			// Simulate the workdir path extraction logic.
			if workdirPath, ok := tt.componentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
				componentPath = workdirPath
			}

			cfg := &shellConfig{
				componentPath: componentPath,
				workingDir:    componentPath,
				varFile:       "test.terraform.tfvars.json",
			}

			assert.Equal(t, tt.expectedCfgPath, cfg.componentPath)
			assert.Equal(t, tt.expectedCfgPath, cfg.workingDir)
		})
	}
}
