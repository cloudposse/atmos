package exec

// terraform_execute_helpers_recovery_test.go covers smart init's plan/apply-time recovery
// path added by this patch:
//   - executeTerraformInitForced (terraform_execute_helpers.go): the forced-reinit helper
//     recoverFromInitRequired calls after a plan/apply-time "init is required" diagnostic.
//   - recoverFromInitRequired / retryMainCommand (terraform_execute_helpers_exec.go): the
//     classify -> force-init -> retry-once orchestration itself.
//
// These use the same seams already established elsewhere in this package:
// executeTerraformInitForcedFn / executeMainTerraformCommandFn function-variable injection
// (see TestExecuteMainTerraformCommand_ExplicitInitDispatchesAfterInit), and the test binary
// itself as a portable fake terraform/tofu subprocess (os.Executable() + testmain_test.go env
// vars) for executeTerraformInitForced's own subprocess-launching behavior.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	autoinit "github.com/cloudposse/atmos/pkg/terraform/autoinit"
)

// ──────────────────────────────────────────────────────────────────────────────
// executeTerraformInitForced
// ──────────────────────────────────────────────────────────────────────────────

// TestExecuteTerraformInitForced_Success verifies that a successful forced reinit
// builds the right -reconfigure/-upgrade argument list, records the smart-init marker
// (so a later invocation can skip a redundant init), and returns no error.
func TestExecuteTerraformInitForced_Success(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	componentPath := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args.txt")

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		Command:          exePath,
		ComponentEnvList: []string{"_ATMOS_TEST_ARGS_FILE=" + argsFile},
	}

	initErr := executeTerraformInitForced(&atmosConfig, info, componentPath, "vars.tfvars", true, true)
	require.NoError(t, initErr)

	argsBytes, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Equal(t, "init\n-reconfigure\n-upgrade", string(argsBytes),
		"forced init must request exactly the reconfigure/upgrade flags recovery determined were needed")

	markerPath := autoinit.MarkerPath(autoinit.DataDir(componentPath, nil))
	_, statErr := os.Stat(markerPath)
	assert.NoError(t, statErr, "a successful forced init must record the smart-init marker, same as an implicit init")
}

// TestExecuteTerraformInitForced_PassVarsAddsVarFileFlag verifies the
// components.terraform.init.pass_vars: true branch appends -var-file with the given path.
func TestExecuteTerraformInitForced_PassVarsAddsVarFileFlag(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	componentPath := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args.txt")

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := &schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		Command:          exePath,
		ComponentEnvList: []string{"_ATMOS_TEST_ARGS_FILE=" + argsFile},
	}

	require.NoError(t, executeTerraformInitForced(&atmosConfig, info, componentPath, "vars.tfvars", false, false))

	argsBytes, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Equal(t, "init\n-var-file\nvars.tfvars", string(argsBytes))
}

// TestExecuteTerraformInitForced_SubprocessFailurePropagates verifies that a failed
// forced-reinit subprocess surfaces its error and does not dispatch after-init
// provisioners or record a marker.
func TestExecuteTerraformInitForced_SubprocessFailurePropagates(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	componentPath := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		Command:          exePath,
		ComponentEnvList: []string{"_ATMOS_TEST_EXIT_ONE=1"},
	}

	initErr := executeTerraformInitForced(&atmosConfig, info, componentPath, "vars.tfvars", false, false)
	require.Error(t, initErr)

	markerPath := autoinit.MarkerPath(autoinit.DataDir(componentPath, nil))
	_, statErr := os.Stat(markerPath)
	assert.True(t, os.IsNotExist(statErr), "a failed forced init must not record a marker")
}

// ──────────────────────────────────────────────────────────────────────────────
// recoverFromInitRequired
// ──────────────────────────────────────────────────────────────────────────────

// withForcedInitSeam swaps executeTerraformInitForcedFn for the duration of the test and
// restores the original on cleanup, mirroring the dispatchAfterInitFn seam pattern already
// used in terraform_execute_helpers_workspace_test.go.
func withForcedInitSeam(t *testing.T, fake func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error) {
	t.Helper()
	original := executeTerraformInitForcedFn
	t.Cleanup(func() { executeTerraformInitForcedFn = original })
	executeTerraformInitForcedFn = fake
}

// withMainCommandSeam swaps executeMainTerraformCommandFn (retryMainCommand's own seam) for
// the duration of the test.
func withMainCommandSeam(t *testing.T, fake func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, []string, string, bool, ...ShellCommandOption) error) {
	t.Helper()
	original := executeMainTerraformCommandFn
	t.Cleanup(func() { executeMainTerraformCommandFn = original })
	executeMainTerraformCommandFn = fake
}

// TestRecoverFromInitRequired_InitSubcommandNeverRecovers verifies that a failed `terraform
// init` itself never enters plan/apply-time recovery -- there is no "init required" fallback
// for init.
func TestRecoverFromInitRequired_InitSubcommandNeverRecovers(t *testing.T) {
	withForcedInitSeam(t, func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
		t.Fatal("forced init must never run for a failed init subcommand itself")
		return nil
	})

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{SubCommand: subcommandInit}
	execCtx := &componentExecContext{varFile: "vars.tfvars"}
	mainErr := errors.New("init failed")
	var stdoutBuf, stderrBuf bytes.Buffer

	err := recoverFromInitRequired(&atmosConfig, info, execCtx, nil, "/tmp/component", false, mainErr, &stdoutBuf, &stderrBuf)
	require.Error(t, err)
	assert.Same(t, mainErr, err)
}

// TestRecoverFromInitRequired_NoDiagnosisReturnsErrUnchanged verifies that output with no
// recognized "init required" signature returns the original error without forcing an init.
func TestRecoverFromInitRequired_NoDiagnosisReturnsErrUnchanged(t *testing.T) {
	withForcedInitSeam(t, func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
		t.Fatal("forced init must not run without a diagnosed init-required signature")
		return nil
	})

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}
	execCtx := &componentExecContext{varFile: "vars.tfvars"}
	mainErr := errors.New("some unrelated validation failure")
	stdoutBuf := bytes.NewBufferString("Error: some unrelated validation failure")
	var stderrBuf bytes.Buffer

	err := recoverFromInitRequired(&atmosConfig, info, execCtx, nil, "/tmp/component", false, mainErr, stdoutBuf, &stderrBuf)
	require.Error(t, err)
	assert.Same(t, mainErr, err)
}

// TestRecoverFromInitRequired_OptedOutJoinsPolicyError verifies that when Atmos's own
// opt-out signals apply (--skip-init here), the policy error from ShouldRecover is joined
// with the original failure instead of silently forcing an init.
func TestRecoverFromInitRequired_OptedOutJoinsPolicyError(t *testing.T) {
	withForcedInitSeam(t, func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
		t.Fatal("forced init must not run when the caller opted out")
		return nil
	})

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan", SkipInit: true}
	execCtx := &componentExecContext{varFile: "vars.tfvars"}
	mainErr := errors.New("plan failed")
	stdoutBuf := bytes.NewBufferString("Error: Backend configuration changed")
	var stderrBuf bytes.Buffer

	err := recoverFromInitRequired(&atmosConfig, info, execCtx, nil, "/tmp/component", false, mainErr, stdoutBuf, &stderrBuf)
	require.Error(t, err)
	assert.True(t, errors.Is(err, mainErr))
}

// TestRecoverFromInitRequired_ForcedInitFailureJoinsError verifies that a failed forced
// reinit is joined with the original main-command error, and that no retry is attempted.
func TestRecoverFromInitRequired_ForcedInitFailureJoinsError(t *testing.T) {
	forcedInitErr := errors.New("forced init failed")
	withForcedInitSeam(t, func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
		return forcedInitErr
	})
	withMainCommandSeam(t, func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, []string, string, bool, ...ShellCommandOption) error {
		t.Fatal("retry must not run when the forced init itself failed")
		return nil
	})

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}
	execCtx := &componentExecContext{varFile: "vars.tfvars"}
	mainErr := errors.New("plan failed")
	stdoutBuf := bytes.NewBufferString("Error: Backend configuration changed")
	var stderrBuf bytes.Buffer

	err := recoverFromInitRequired(&atmosConfig, info, execCtx, nil, "/tmp/component", false, mainErr, stdoutBuf, &stderrBuf)
	require.Error(t, err)
	assert.True(t, errors.Is(err, mainErr))
	assert.True(t, errors.Is(err, forcedInitErr))
}

// TestRecoverFromInitRequired_SuccessfulRecoveryRetriesMainCommand verifies the full happy
// path: a diagnosed failure forces exactly one reinit (with the flags the diagnostic asked
// for) and then retries the main command exactly once via retryMainCommand.
func TestRecoverFromInitRequired_SuccessfulRecoveryRetriesMainCommand(t *testing.T) {
	var forcedInitCalls int
	withForcedInitSeam(t, func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, componentPath, varFile string, reconfigure, upgrade bool, _ ...ShellCommandOption) error {
		forcedInitCalls++
		assert.Equal(t, "/tmp/component", componentPath)
		assert.Equal(t, "vars.tfvars", varFile)
		assert.True(t, reconfigure)
		return nil
	})

	var retryCalls int
	withMainCommandSeam(t, func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, allArgsAndFlags []string, componentPath string, uploadStatusFlag bool, _ ...ShellCommandOption) error {
		retryCalls++
		assert.Equal(t, []string{"plan", "-out=plan.tfplan"}, allArgsAndFlags)
		assert.Equal(t, "/tmp/component", componentPath)
		assert.False(t, uploadStatusFlag)
		return nil
	})

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}
	execCtx := &componentExecContext{varFile: "vars.tfvars"}
	mainErr := errors.New("plan failed")
	stdoutBuf := bytes.NewBufferString("Error: Backend configuration changed")
	var stderrBuf bytes.Buffer

	err := recoverFromInitRequired(&atmosConfig, info, execCtx, []string{"plan", "-out=plan.tfplan"}, "/tmp/component", false, mainErr, stdoutBuf, &stderrBuf)
	require.NoError(t, err)
	assert.Equal(t, 1, forcedInitCalls)
	assert.Equal(t, 1, retryCalls)
}

// ──────────────────────────────────────────────────────────────────────────────
// retryMainCommand
// ──────────────────────────────────────────────────────────────────────────────

// TestRetryMainCommand_ScopesExecMetadataToRetryOnly verifies that retryMainCommand resets
// info.ExecMetadataRawOutput to reflect only the retry attempt's own captured output, not
// anything accumulated by the original (failed) attempt.
func TestRetryMainCommand_ScopesExecMetadataToRetryOnly(t *testing.T) {
	withMainCommandSeam(t, func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ []string, _ string, _ bool, opts ...ShellCommandOption) error {
		stdoutW, _ := execMetadataOutputCaptureFromOpts(opts...)
		require.NotNil(t, stdoutW, "retryMainCommand must inject its own exec-metadata capture writer")
		_, err := stdoutW.Write([]byte("retry attempt output"))
		require.NoError(t, err)
		return nil
	})

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		SubCommand:            "plan",
		ExecMetadataRawOutput: "stale output from the original failed attempt",
	}

	err := retryMainCommand(&atmosConfig, info, []string{"plan"}, "/tmp/component", false)
	require.NoError(t, err)
	assert.Equal(t, "retry attempt output", info.ExecMetadataRawOutput,
		"ExecMetadataRawOutput must reflect only the retry, not the original attempt's stale output")
}

// TestRetryMainCommand_PropagatesRetryError verifies that a failed retry attempt's error
// propagates unchanged.
func TestRetryMainCommand_PropagatesRetryError(t *testing.T) {
	retryErr := errors.New("still failing after retry")
	withMainCommandSeam(t, func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, []string, string, bool, ...ShellCommandOption) error {
		return retryErr
	})

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}

	err := retryMainCommand(&atmosConfig, info, []string{"plan"}, "/tmp/component", false)
	require.Error(t, err)
	assert.Same(t, retryErr, err)
}
