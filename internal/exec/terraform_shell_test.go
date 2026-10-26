package exec

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// errTestInitFailed is a sentinel used to assert init-failure propagation in the shell lifecycle.
var errTestInitFailed = errors.New("init failed")

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

func TestShellInfoFromOptions_AllFieldsMapped(t *testing.T) {
	// Ensure all relevant fields from ShellOptions are properly mapped to ConfigAndStacksInfo.
	// Note: ProcessingOptions (ProcessTemplates, ProcessFunctions, Skip) are NOT mapped here;
	// they are passed directly to ProcessStacks in ExecuteTerraformShell.
	opts := &ShellOptions{
		Component: "my-component",
		Stack:     "my-stack",
		DryRun:    true,
		Identity:  "my-identity",
	}

	info := shellInfoFromOptions(opts)

	// Verify all mapped fields.
	assert.Equal(t, "my-component", info.ComponentFromArg)
	assert.Equal(t, "my-stack", info.Stack)
	assert.Equal(t, "my-stack", info.StackFromArg)
	assert.Equal(t, "terraform", info.ComponentType)
	assert.Equal(t, "shell", info.SubCommand)
	assert.True(t, info.DryRun)
	assert.Equal(t, "my-identity", info.Identity)
}

func TestResolveWorkdirPath_ValidWorkdir(t *testing.T) {
	// Test the valid workdir path branch explicitly.
	workdirPath := "/custom/workdir/path"
	componentSection := map[string]any{
		provWorkdir.WorkdirPathKey: workdirPath,
	}
	originalPath := "/original/component/path"

	result := resolveWorkdirPath(componentSection, originalPath)

	assert.Equal(t, workdirPath, result)
	assert.NotEqual(t, originalPath, result)
}

func TestResolveWorkdirPath_EmptyMap(t *testing.T) {
	// Empty map should return original path.
	result := resolveWorkdirPath(map[string]any{}, "/original/path")
	assert.Equal(t, "/original/path", result)
}

func TestShellOptionsForUI_DefaultProcessingOptions(t *testing.T) {
	// Verify that UI shell options have correct default processing options.
	opts := shellOptionsForUI("vpc", "dev")

	assert.True(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
	assert.Empty(t, opts.Skip)
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

func TestApplyShellSecretEnv(t *testing.T) {
	const secret = "shell-secret-value-9f8e7d"
	iolib.RegisterSecret(secret)

	newInfo := func() *schema.ConfigAndStacksInfo {
		info := &schema.ConfigAndStacksInfo{
			ComponentVarsSection: map[string]any{
				"token":  secret,
				"region": "us-east-1-shellsecret",
			},
		}
		computeTerraformSecretVarKeys(info)
		require.True(t, info.TerraformSecretVarKeys["token"], "secret var should be flagged")
		require.False(t, info.TerraformSecretVarKeys["region"], "non-secret var should not be flagged")
		return info
	}

	t.Run("with-secrets exports TF_VAR_", func(t *testing.T) {
		info := newInfo()
		require.NoError(t, applyShellSecretEnv(info, true))

		var found bool
		for _, e := range info.ComponentEnvList {
			if e == "TF_VAR_token="+secret {
				found = true
			}
			// The non-secret var must never be exported via env here.
			assert.NotContains(t, e, "TF_VAR_region")
		}
		assert.True(t, found, "expected TF_VAR_token to be exported into the shell env")
	})

	t.Run("without-secrets withholds TF_VAR_", func(t *testing.T) {
		info := newInfo()
		require.NoError(t, applyShellSecretEnv(info, false))

		for _, e := range info.ComponentEnvList {
			assert.NotContains(t, e, "TF_VAR_token", "secret must not be exported without --with-secrets")
		}
	})
}

// TestShellInfoFromOptions_MapsSkipInit verifies the --skip-init flag value flows from
// ShellOptions into ConfigAndStacksInfo so shouldRunTerraformInit can honor it.
func TestShellInfoFromOptions_MapsSkipInit(t *testing.T) {
	t.Run("skip-init true", func(t *testing.T) {
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev", SkipInit: true})
		assert.True(t, info.SkipInit)
	})
	t.Run("skip-init false (default)", func(t *testing.T) {
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
		assert.False(t, info.SkipInit)
	})
}

// TestShouldRunTerraformInit_Shell guards the regression: the shell subcommand must run
// `terraform init` by default, and --skip-init must disable it.
func TestShouldRunTerraformInit_Shell(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("shell runs init by default", func(t *testing.T) {
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
		assert.True(t, shouldRunTerraformInit(atmosConfig, &info),
			"terraform shell must run init by default (pre-v1.202.0 behavior)")
	})

	t.Run("--skip-init disables init", func(t *testing.T) {
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev", SkipInit: true})
		assert.False(t, shouldRunTerraformInit(atmosConfig, &info))
	})
}

// TestShouldSkipWorkspaceSetup_Shell guards the regression: the shell subcommand must NOT skip
// workspace selection (so the user lands in the component's workspace, not `default`), except for
// the http backend or when TF_WORKSPACE is already set.
func TestShouldSkipWorkspaceSetup_Shell(t *testing.T) {
	t.Run("shell selects workspace", func(t *testing.T) {
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
		assert.False(t, shouldSkipWorkspaceSetup(&info),
			"terraform shell must select/create the workspace (pre-v1.202.0 behavior)")
	})

	t.Run("http backend skips workspace", func(t *testing.T) {
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
		info.ComponentBackendType = "http"
		assert.True(t, shouldSkipWorkspaceSetup(&info))
	})

	t.Run("TF_WORKSPACE set skips workspace", func(t *testing.T) {
		t.Setenv("TF_WORKSPACE", "dev")
		info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
		assert.True(t, shouldSkipWorkspaceSetup(&info))
	})
}

// swapShellLifecycleSeams overrides the shell-lifecycle function seams with recorders and restores
// them on cleanup. Returns a pointer to the ordered list of recorded step names.
func swapShellLifecycleSeams(t *testing.T) *[]string {
	t.Helper()
	calls := &[]string{}

	origInit, origWs, origExec := shellInitFn, shellWorkspaceFn, shellExecFn
	t.Cleanup(func() {
		shellInitFn, shellWorkspaceFn, shellExecFn = origInit, origWs, origExec
	})

	shellInitFn = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _, _ string, _ ...ShellCommandOption) error {
		*calls = append(*calls, "init")
		return nil
	}
	shellWorkspaceFn = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ string, _ ...ShellCommandOption) error {
		*calls = append(*calls, "workspace")
		return nil
	}
	shellExecFn = func(_ *schema.AtmosConfiguration, _, _ string, _ []string, _, _, _, _ string) error {
		*calls = append(*calls, "shell")
		return nil
	}
	return calls
}

// TestExecuteShellLifecycle_RunsInitThenWorkspaceThenShell is the core regression test: by default
// `terraform shell` must run init, then select/create the workspace, then launch the shell — in
// that order. Against the v1.202.0–v1.279.x code the init and workspace steps were missing.
func TestExecuteShellLifecycle_RunsInitThenWorkspaceThenShell(t *testing.T) {
	calls := swapShellLifecycleSeams(t)

	atmosConfig := &schema.AtmosConfiguration{}
	info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
	cfg := &shellConfig{componentPath: t.TempDir(), workingDir: t.TempDir(), varFile: "dev-vpc.tfvars.json"}

	require.NoError(t, executeShellLifecycle(atmosConfig, &info, cfg))
	assert.Equal(t, []string{"init", "workspace", "shell"}, *calls)
}

// TestExecuteShellLifecycle_SkipInit_KeepsWorkspace verifies the levers are decoupled: --skip-init
// suppresses init but the workspace is still selected before the shell launches.
func TestExecuteShellLifecycle_SkipInit_KeepsWorkspace(t *testing.T) {
	calls := swapShellLifecycleSeams(t)

	atmosConfig := &schema.AtmosConfiguration{}
	info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev", SkipInit: true})
	cfg := &shellConfig{componentPath: t.TempDir(), workingDir: t.TempDir(), varFile: "dev-vpc.tfvars.json"}

	require.NoError(t, executeShellLifecycle(atmosConfig, &info, cfg))
	assert.Equal(t, []string{"workspace", "shell"}, *calls,
		"--skip-init must skip init but still select the workspace and launch the shell")
}

// newShellPrepInfo builds a minimal ConfigAndStacksInfo whose component working dir resolves to
// tmpDir (via the workdir provisioner key), so prepareShellExecution writes its varfile there and
// generateConfigFiles no-ops (no backend/providers configured).
func newShellPrepInfo(tmpDir string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		FinalComponent:   "vpc",
		ComponentType:    "terraform",
		Stack:            "dev",
		ComponentSection: map[string]any{provWorkdir.WorkdirPathKey: tmpDir},
		ComponentVarsSection: map[string]any{
			"name": "vpc-test",
		},
	}
}

// TestPrepareShellExecution_HappyPath exercises the full per-component setup the shell performs
// before init/workspace/shell: binary + toolchain resolution, secret-key computation, disk-safe
// varfile write, backend/provider config generation, env assembly, and RC cleanup registration.
func TestPrepareShellExecution_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := newShellPrepInfo(tmpDir)
	cfg := &shellConfig{componentPath: tmpDir, workingDir: tmpDir, varFile: "dev-vpc.terraform.tfvars.json"}

	err := prepareShellExecution(atmosConfig, info, cfg, false)
	require.NoError(t, err)

	// RC cleanup is registered so the temporary Terraform CLI config survives init/workspace/shell;
	// run it here to avoid leaking the temp file.
	if info.RCCleanup != nil {
		t.Cleanup(func() { _ = info.RCCleanup() })
	}

	// The terraform binary was resolved.
	assert.NotEmpty(t, info.Command, "terraform/tofu command must be resolved")

	// The disk-safe varfile was written into the resolved working dir.
	varfilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
	assert.FileExists(t, varfilePath, "disk-safe varfile must be written")

	// Standard Atmos env vars were assembled for the subprocess/shell.
	assert.Contains(t, info.ComponentEnvList, "TF_IN_AUTOMATION=true",
		"component env must be assembled before launching the shell")
}

// TestPrepareShellExecution_WithSecrets routes a secret-bearing var through the shell env when
// --with-secrets is set: it must be exported as TF_VAR_* but never written to the on-disk varfile.
func TestPrepareShellExecution_WithSecrets(t *testing.T) {
	const secret = "prepare-shell-secret-7a6b5c"
	iolib.RegisterSecret(secret)

	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := newShellPrepInfo(tmpDir)
	info.ComponentVarsSection = map[string]any{"token": secret, "region": "us-east-1-prepareshell"}
	cfg := &shellConfig{componentPath: tmpDir, workingDir: tmpDir, varFile: "dev-vpc.terraform.tfvars.json"}

	require.NoError(t, prepareShellExecution(atmosConfig, info, cfg, true))
	if info.RCCleanup != nil {
		t.Cleanup(func() { _ = info.RCCleanup() })
	}

	// The secret is exported into the shell env as TF_VAR_token.
	var exported bool
	for _, e := range info.ComponentEnvList {
		if e == "TF_VAR_token="+secret {
			exported = true
		}
	}
	assert.True(t, exported, "with --with-secrets the secret must be exported as TF_VAR_token")

	// The secret must NOT be written to the on-disk varfile.
	varfilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
	data, err := os.ReadFile(varfilePath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), secret, "secret must never be written to the on-disk varfile")
}

// TestPrepareShellExecution_VarfileWriteError verifies an error from writing the varfile is
// propagated. The working dir resolves to a path nested under a regular file, so the write fails.
func TestPrepareShellExecution_VarfileWriteError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file, then point the working dir at a path *inside* it so the varfile
	// write (and its parent-dir creation) fails.
	blocker := filepath.Join(tmpDir, "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	badDir := filepath.Join(blocker, "sub")

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := newShellPrepInfo(badDir)
	cfg := &shellConfig{componentPath: badDir, workingDir: badDir, varFile: "dev-vpc.terraform.tfvars.json"}

	err := prepareShellExecution(atmosConfig, info, cfg, false)
	require.Error(t, err, "varfile write into a non-directory path must fail")
}

// TestExecuteShellLifecycle_WorkdirComponent_SkipsWorkspaceClean covers the workdir branch: when
// the component is provisioned into a workdir, executeShellLifecycle skips the .terraform/environment
// cleanup but still runs init -> workspace -> shell in order.
func TestExecuteShellLifecycle_WorkdirComponent_SkipsWorkspaceClean(t *testing.T) {
	calls := swapShellLifecycleSeams(t)

	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{}
	info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
	info.ComponentSection = map[string]any{provWorkdir.WorkdirPathKey: tmpDir}
	cfg := &shellConfig{componentPath: tmpDir, workingDir: tmpDir, varFile: "dev-vpc.tfvars.json"}

	require.NoError(t, executeShellLifecycle(atmosConfig, &info, cfg))
	assert.Equal(t, []string{"init", "workspace", "shell"}, *calls,
		"workdir components still run init, workspace, and shell in order")
}

// TestExecuteTerraformInitCommand_DryRun covers the provisioner-free init helper used by the shell:
// in dry-run mode it builds the init args and dispatches after.terraform.init without launching a
// real subprocess, returning no error.
func TestExecuteTerraformInitCommand_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		FinalComponent:   "vpc",
		ComponentType:    "terraform",
		Stack:            "dev",
		Command:          "terraform",
		DryRun:           true,
		ComponentSection: map[string]any{},
	}

	err := executeTerraformInitCommand(atmosConfig, info, tmpDir, "dev-vpc.terraform.tfvars.json")
	require.NoError(t, err, "dry-run init must not launch a subprocess and must not error")
}

// TestRunShellSession_PreparesThenRunsLifecycle covers the post-ProcessStacks glue of the shell
// entry point: prepareShellExecution runs, the RC cleanup is registered, and the lifecycle
// (init -> workspace -> shell) runs in order. The lifecycle subprocess/shell steps are stubbed.
func TestRunShellSession_PreparesThenRunsLifecycle(t *testing.T) {
	calls := swapShellLifecycleSeams(t)

	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := newShellPrepInfo(tmpDir)
	cfg := &shellConfig{componentPath: tmpDir, workingDir: tmpDir, varFile: "dev-vpc.terraform.tfvars.json"}

	require.NoError(t, runShellSession(atmosConfig, info, cfg, false))

	// prepareShellExecution ran (binary resolved, env assembled).
	assert.NotEmpty(t, info.Command, "prepareShellExecution must resolve the terraform binary")
	assert.Contains(t, info.ComponentEnvList, "TF_IN_AUTOMATION=true")
	// The lifecycle ran in order.
	assert.Equal(t, []string{"init", "workspace", "shell"}, *calls)
}

// TestRunShellSession_PrepareErrorSkipsLifecycle verifies that when prepareShellExecution fails,
// the lifecycle (init/workspace/shell) never runs.
func TestRunShellSession_PrepareErrorSkipsLifecycle(t *testing.T) {
	calls := swapShellLifecycleSeams(t)

	tmpDir := t.TempDir()
	// Point the working dir at a path nested under a regular file so the varfile write fails.
	blocker := filepath.Join(tmpDir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	badDir := filepath.Join(blocker, "sub")

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := newShellPrepInfo(badDir)
	cfg := &shellConfig{componentPath: badDir, workingDir: badDir, varFile: "dev-vpc.terraform.tfvars.json"}

	err := runShellSession(atmosConfig, info, cfg, false)
	require.Error(t, err, "prepare failure must propagate")
	assert.Empty(t, *calls, "the init/workspace/shell lifecycle must not run when prepare fails")
}

// TestExecuteShellLifecycle_WorkspaceErrorStopsBeforeShell verifies a workspace-setup failure
// aborts the lifecycle (after init) before the interactive shell is launched.
func TestExecuteShellLifecycle_WorkspaceErrorStopsBeforeShell(t *testing.T) {
	calls := swapShellLifecycleSeams(t)
	wantErr := errors.New("workspace failed")
	shellWorkspaceFn = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ string, _ ...ShellCommandOption) error {
		*calls = append(*calls, "workspace")
		return wantErr
	}

	atmosConfig := &schema.AtmosConfiguration{}
	info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
	cfg := &shellConfig{componentPath: t.TempDir(), workingDir: t.TempDir(), varFile: "dev-vpc.tfvars.json"}

	err := executeShellLifecycle(atmosConfig, &info, cfg)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, []string{"init", "workspace"}, *calls, "shell must not launch after workspace setup fails")
}

// TestExecuteTerraformInitPhase_DryRun covers the standard init pre-step (prepareInitExecution +
// executeTerraformInitCommand) in dry-run mode: it returns the resolved path without launching a
// real subprocess.
func TestExecuteTerraformInitPhase_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		FinalComponent:   "vpc",
		ComponentType:    "terraform",
		Stack:            "dev",
		Command:          "terraform",
		DryRun:           true,
		ComponentSection: map[string]any{provWorkdir.WorkdirPathKey: tmpDir},
	}

	newPath, err := executeTerraformInitPhase(atmosConfig, info, tmpDir, "dev-vpc.terraform.tfvars.json")
	require.NoError(t, err)
	assert.Equal(t, tmpDir, newPath, "workdir components return the workdir path")
}

// TestExecuteTerraformInitPhase_SubprocessError covers the init error path: when the terraform
// binary cannot be executed, both executeTerraformInitPhase and executeTerraformInitCommand
// propagate the failure (no dry-run, so the subprocess actually runs and fails fast).
func TestExecuteTerraformInitPhase_SubprocessError(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		FinalComponent:   "vpc",
		ComponentType:    "terraform",
		Stack:            "dev",
		// A binary that does not exist on any platform, so `init` fails fast without a real terraform.
		Command:          "atmos-nonexistent-terraform-binary-for-tests",
		DryRun:           false,
		ComponentSection: map[string]any{provWorkdir.WorkdirPathKey: tmpDir},
	}

	_, err := executeTerraformInitPhase(atmosConfig, info, tmpDir, "dev-vpc.terraform.tfvars.json")
	require.Error(t, err, "init must fail when the terraform binary cannot be executed")
}

// TestExecuteShellLifecycle_InitErrorStopsBeforeShell verifies an init failure aborts the
// lifecycle before the workspace is touched or the shell is launched.
func TestExecuteShellLifecycle_InitErrorStopsBeforeShell(t *testing.T) {
	calls := swapShellLifecycleSeams(t)
	wantErr := errTestInitFailed
	shellInitFn = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _, _ string, _ ...ShellCommandOption) error {
		*calls = append(*calls, "init")
		return wantErr
	}

	atmosConfig := &schema.AtmosConfiguration{}
	info := shellInfoFromOptions(&ShellOptions{Component: "vpc", Stack: "dev"})
	cfg := &shellConfig{componentPath: t.TempDir(), workingDir: t.TempDir(), varFile: "dev-vpc.tfvars.json"}

	err := executeShellLifecycle(atmosConfig, &info, cfg)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, []string{"init"}, *calls, "workspace and shell must not run after init fails")
}
