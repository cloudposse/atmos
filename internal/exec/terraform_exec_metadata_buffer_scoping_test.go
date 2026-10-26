package exec

// terraform_exec_metadata_buffer_scoping_test.go is the reproduce-first
// regression test for FR-006f (research.md Decision 32): the exec-metadata
// parser's input must be scoped to only the final plan/apply/deploy
// subprocess invocation, not the combined init+workspace-select+main buffer
// other consumers (e.g. cmd/terraform's capturedPlanOutput, used by CI
// job-summary hooks) legitimately need.

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteCommandPipeline_ExecMetadataOutput_NotPoisonedByWorkspaceSelect
// reproduces the exact reported production bug: workspace-select's own
// incidental output contains a "No changes." lookalike string, but the main
// plan invocation's own output has a real "Plan: N to add..." summary. The
// info.ExecMetadataRawOutput field (the exec-metadata parser's input) must
// reflect only the main command's own output — never workspace-select's — while a
// caller-supplied WithStdoutCapture buffer (mirroring cmd/terraform's
// capturedPlanOutput) still legitimately accumulates both phases combined.
//
// Must not run in parallel — sets os.Stdin = nil (global state, mirrors the
// existing TestExecuteCommandPipeline_TTYError/TestExecuteCommandPipeline_SingleInvocation
// pattern in this package).
func TestExecuteCommandPipeline_ExecMetadataOutput_NotPoisonedByWorkspaceSelect(t *testing.T) {
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })

	// Ensure shouldSkipWorkspaceSetup's TF_WORKSPACE check doesn't accidentally
	// skip workspace setup in an environment where that var happens to be set.
	t.Setenv("TF_WORKSPACE", "")

	exePath, err := os.Executable()
	require.NoError(t, err)

	const selectOutput = "Workspace selected. No changes. Your infrastructure matches the configuration."
	const planOutput = "Plan: 2 to add, 0 to change, 0 to destroy."

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:         "plan",
		SkipInit:           true, // init isn't needed to reproduce this bug; workspace-select alone suffices.
		Command:            exePath,
		TerraformWorkspace: "dev",
		ComponentEnvList: []string{
			testEnvFakeTerraform + "=1",
			testEnvFakeTerraformSelectOutput + "=" + selectOutput,
			testEnvFakeTerraformPlanOutput + "=" + planOutput,
		},
	}
	execCtx := &componentExecContext{
		componentPath: t.TempDir(),
		varFile:       "",
		planFile:      "",
		workingDir:    t.TempDir(),
	}

	// Mirror cmd/terraform's own stdout capture, which legitimately needs the
	// WHOLE pipeline's output (capturedPlanOutput, used by CI job-summary
	// hooks) — this must still see both phases combined, unaffected by the
	// exec-metadata scoping fix.
	var wholePipelineStdout bytes.Buffer
	err = executeCommandPipeline(&atmosConfig, &info, execCtx, WithStdoutCapture(&wholePipelineStdout))
	require.NoError(t, err)

	assert.Contains(t, info.ExecMetadataRawOutput, planOutput,
		"exec-metadata output must contain the main plan command's own real summary")
	assert.NotContains(t, info.ExecMetadataRawOutput, "Workspace selected",
		"exec-metadata output must NOT contain workspace-select's incidental output")
	assert.NotContains(t, info.ExecMetadataRawOutput, "No changes. Your infrastructure matches the configuration.",
		"a 'No changes.' lookalike from workspace-select must not poison the exec-metadata parser's input")

	whole := wholePipelineStdout.String()
	assert.Contains(t, whole, "Workspace selected", "the whole-pipeline buffer other consumers rely on must still see workspace-select's output")
	assert.Contains(t, whole, planOutput, "the whole-pipeline buffer must still see the main command's output too")
}
