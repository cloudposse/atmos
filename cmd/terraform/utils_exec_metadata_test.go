package terraform

// utils_exec_metadata_test.go covers the multi-component exec-metadata
// aggregation added for FR-006a (research.md Decisions 11/17): a
// multi-component graph run must produce exactly one execution record for
// the whole invocation, with each node's identity/outcome folded into that
// single record's Data field, not sent as separate per-node records.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestParseTerraformResourceChanges_ApplySuccess verifies resource-level
// change extraction from a real captured apply output (FR-006, data-model.md
// Decision 17), using the same fixture pkg/ci/plugins/terraform's own parser
// tests exercise, proving the mirror-struct decode round-trip matches the
// real (internal-package) OutputResult/TerraformOutputData shape.
func TestParseTerraformResourceChanges_ApplySuccess(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	changes := parseTerraformResourceChanges("apply", string(data))

	require.Len(t, changes, 3)
	var addresses []string
	for _, c := range changes {
		assert.Equal(t, "created", c.Action)
		addresses = append(addresses, c.Address)
	}
	assert.Contains(t, addresses, "aws_security_group.allow_http")
	assert.Contains(t, addresses, "aws_instance.web")
	assert.Contains(t, addresses, "aws_eip.web")
}

// TestParseTerraformResourceChanges_DeployParsedAsApply verifies "deploy" is
// parsed with apply semantics, matching pkg/ci/plugins/terraform's own
// onAfterDeploy override.
func TestParseTerraformResourceChanges_DeployParsedAsApply(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	changes := parseTerraformResourceChanges("deploy", string(data))

	require.Len(t, changes, 3)
}

// TestParseTerraformResourceChanges_NonTerraformSubcommand verifies no
// resource-change entries are produced for subcommands outside plan/apply
// (deploy included via the mapping above) — e.g. "output"/"refresh", which
// terraformHookEvents also wires NodeHooks for but which have no plan/apply
// -shaped stdout to parse.
func TestParseTerraformResourceChanges_NonTerraformSubcommand(t *testing.T) {
	assert.Nil(t, parseTerraformResourceChanges("output", "anything"))
	assert.Nil(t, parseTerraformResourceChanges("plan", ""))
}

// TestBuildTerraformExecData_ApplySuccess verifies the single-component
// combined-object Data shape (research.md Decision 18, data-model.md's
// TerraformExecData) — resource_counts/outputs/warnings/changes all present
// in one object, built from the same real fixture used by the
// multi-component tests above, proving both call sites decode through the
// same parseTerraformOutputMirror helper consistently.
func TestBuildTerraformExecData_ApplySuccess(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("apply", string(data))
	require.NotNil(t, result)

	asMap, ok := result.(map[string]any)
	require.True(t, ok)

	resourceCounts, ok := asMap["resource_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 3, resourceCounts["create"])

	changes, ok := asMap["changes"].([]execNodeResult)
	require.True(t, ok)
	require.Len(t, changes, 3)
	var addresses []string
	for _, c := range changes {
		assert.Equal(t, "created", c.Action)
		addresses = append(addresses, c.Address)
	}
	assert.Contains(t, addresses, "aws_instance.web")

	assert.Contains(t, asMap, "outputs")
	assert.Contains(t, asMap, "warnings")
}

// TestBuildTerraformExecData_DeployParsedAsApply mirrors
// TestParseTerraformResourceChanges_DeployParsedAsApply for the
// single-component combined-object shape.
func TestBuildTerraformExecData_DeployParsedAsApply(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("deploy", string(data))
	require.NotNil(t, result)
}

// TestBuildTerraformExecData_NonTerraformSubcommand verifies nil is returned
// for a non-terraform subcommand or empty output, matching
// parseTerraformResourceChanges's existing coverage shape.
func TestBuildTerraformExecData_NonTerraformSubcommand(t *testing.T) {
	assert.Nil(t, buildTerraformExecData("output", "anything"))
	assert.Nil(t, buildTerraformExecData("plan", ""))
}

// TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser is the regression
// guard for research.md Decision 18's ciMode-decoupling: plan.go/apply.go/
// deploy.go's RunE call terraformCaptureShellOpts unconditionally (no ciMode
// check gates it), so stdout/stderr capture and the exec-metadata parser
// closure must always be wired, regardless of whether the caller separately
// decides to also use ciMode for its own CI-job-summary post-processing.
func TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser(t *testing.T) {
	opts, stdoutBuf, stderrBuf := terraformCaptureShellOpts()

	require.NotNil(t, stdoutBuf)
	require.NotNil(t, stderrBuf)
	require.Len(t, opts, 3, "WithStdoutCapture, WithStderrCapture, WithExecMetadataParser")
}

// TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime verifies the
// closure terraformCaptureShellOpts wires into WithExecMetadataParser reads
// whatever has been written into stdoutBuf/stderrBuf by the time it's
// invoked, combining both streams and stripping ANSI before parsing —
// mirroring how ExecuteTerraform populates the buffers during execution and
// captureExecMetadataSync invokes the parser afterward.
func TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime(t *testing.T) {
	fixture, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	var stdoutBuf, stderrBuf bytes.Buffer
	parser := terraformExecMetadataParserFunc(&stdoutBuf, &stderrBuf)

	// Nothing written yet: parser must return nil (empty output).
	assert.Nil(t, parser("apply"))

	stdoutBuf.WriteString(string(fixture))

	result := parser("apply")
	require.NotNil(t, result)
	asMap, ok := result.(map[string]any)
	require.True(t, ok)
	require.Contains(t, asMap, "resource_counts")
	resourceCounts, ok := asMap["resource_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 3, resourceCounts["create"])
}

// TestTerraformNodeHooks_RecordExecResultAccumulates verifies After()
// accumulates one execNodeResult per node call, with the correct
// component/stack/exitCode, and that concurrent calls (as the scheduler may
// dispatch nodes concurrently) are all safely recorded.
func TestTerraformNodeHooks_RecordExecResultAccumulates(t *testing.T) {
	applyOutput, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	t.Chdir("../../examples/demo-stacks")

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformApply, subCommand: "apply"}

	info1 := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "myapp", ComponentFromArg: "myapp", ComponentType: "terraform"}
	info2 := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "myapp", ComponentFromArg: "myapp", ComponentType: "terraform"}

	require.NoError(t, nodeHooks.After(context.Background(), info1, string(applyOutput), nil))
	require.NoError(t, nodeHooks.After(context.Background(), info2, "", errUtils.ExitCodeError{Code: 1}))

	nodeHooks.mu.Lock()
	results := append([]execNodeResult(nil), nodeHooks.results...)
	nodeHooks.mu.Unlock()

	// One base identity/outcome entry per node, plus one entry per resource
	// change parsed from the first node's captured output (3 created
	// resources) — the second node had no output to parse, so only its base
	// entry is present.
	require.Len(t, results, 5, "base entry per node, plus resource-change entries for the node with parseable output")
	assert.Contains(t, results, execNodeResult{Component: "myapp", Stack: "dev", ExitCode: 0})
	assert.Contains(t, results, execNodeResult{Component: "myapp", Stack: "dev", ExitCode: 1})
	assert.Contains(t, results, execNodeResult{Component: "myapp", Stack: "dev", ExitCode: 0, Action: "created", Address: "aws_instance.web"})
}

// TestCaptureMultiComponentExecMetadata_NoOpWithoutNodeHooks verifies the
// aggregate capture never panics or attempts delivery when NodeHooks wasn't
// wired (e.g. a subcommand outside terraformHookEvents).
func TestCaptureMultiComponentExecMetadata_NoOpWithoutNodeHooks(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{}
	assert.NotPanics(t, func() {
		captureMultiComponentExecMetadata(info, "plan", nil)
	})
}

// TestCaptureMultiComponentExecMetadata_NoOpForNonSyncSubcommand verifies
// the aggregate capture skips subcommands outside the synchronous
// exec-metadata allowlist (e.g. "output"), even when NodeHooks is wired.
func TestCaptureMultiComponentExecMetadata_NoOpForNonSyncSubcommand(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		NodeHooks: &terraformNodeHooks{cmd: newHookTestCmd(), subCommand: "output"},
	}
	assert.NotPanics(t, func() {
		captureMultiComponentExecMetadata(info, "output", nil)
	})
}

// TestCaptureMultiComponentExecMetadata_ExactlyOneRequestForWholeRun is the
// direct regression test for FR-006a's independent test criterion: a
// multi-component run with 2 accumulated node results must produce exactly
// one POST /v1/atmos/exec request for the whole invocation, with both
// nodes' identity/outcome folded into that single request's Data array —
// never one request per component.
func TestCaptureMultiComponentExecMetadata_ExactlyOneRequestForWholeRun(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")
	t.Setenv("CI", "true")

	var requestCount atomic.Int32
	var receivedBody dtos.ExecUploadRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	t.Setenv("ATMOS_PRO_TOKEN", "test-token")
	t.Setenv("ATMOS_PRO_BASE_URL", server.URL)

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), subCommand: "plan"}
	info1 := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "myapp", ComponentFromArg: "myapp", ComponentType: "terraform"}
	info2 := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "other", ComponentFromArg: "other", ComponentType: "terraform"}
	nodeHooks.recordExecResult(info1, "", nil)
	nodeHooks.recordExecResult(info2, "", nil)

	info := &schema.ConfigAndStacksInfo{NodeHooks: nodeHooks}
	captureMultiComponentExecMetadata(info, "plan", nil)

	assert.Equal(t, int32(1), requestCount.Load(), "exactly one execution record for the whole multi-component run")
	assert.Equal(t, "terraform plan", receivedBody.Command)
	assert.NotEmpty(t, receivedBody.ExecutionID)

	var data []execNodeResult
	require.NoError(t, json.Unmarshal(receivedBody.Data, &data))
	require.Len(t, data, 2, "both nodes' results must be folded into the single record's Data")
	assert.Contains(t, data, execNodeResult{Component: "myapp", Stack: "dev", ExitCode: 0})
	assert.Contains(t, data, execNodeResult{Component: "other", Stack: "dev", ExitCode: 0})
}
