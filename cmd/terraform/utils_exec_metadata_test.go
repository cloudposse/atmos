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
	iolib "github.com/cloudposse/atmos/pkg/io"
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
// combined-object Data shape (research.md Decisions 18/19/20/21,
// data-model.md's TerraformExecData) — resource_counts/outputs/warnings/
// changes/has_changes/has_errors/errors/version all present in one object,
// built from the same real fixture used by the multi-component tests above,
// proving both call sites decode through the same parseTerraformOutputMirror
// helper consistently.
func TestBuildTerraformExecData_ApplySuccess(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("apply", string(data), "web", "plat-use2-dev", 0)
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

	require.Contains(t, asMap, "outputs")
	outputs, ok := asMap["outputs"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, outputs, "instance_id")
	instanceID, ok := outputs["instance_id"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, instanceID["sensitive"])
	assert.Equal(t, "i-12345678", instanceID["value"])

	assert.Contains(t, asMap, "warnings")
	assert.Equal(t, true, asMap["has_changes"])
	assert.Equal(t, false, asMap["has_errors"])
	assert.Empty(t, asMap["errors"])
	assert.Equal(t, terraformExecDataVersion, asMap["version"])
	assert.Equal(t, 0, asMap["exit_code"])
	assert.Equal(t, "web", asMap["component"])
	assert.Equal(t, "plat-use2-dev", asMap["stack"])
}

// TestBuildTerraformExecData_ApplyFailure verifies HasErrors/Errors decode
// correctly for a failing apply (research.md Decision 20), using the
// existing apply_failure.txt fixture.
func TestBuildTerraformExecData_ApplyFailure(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_failure.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("apply", string(data), "", "", 1)
	require.NotNil(t, result)

	asMap, ok := result.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, true, asMap["has_errors"])
	assert.Equal(t, 1, asMap["exit_code"])
	errs, ok := asMap["errors"].([]string)
	require.True(t, ok)
	assert.NotEmpty(t, errs)
}

// TestBuildTerraformExecData_EmptyComponentStackOmitted verifies component/
// stack are absent from the map (not empty strings) when either argument is
// empty — research.md Decision 21's explicit "omission is the clearer
// signal" rule.
func TestBuildTerraformExecData_EmptyComponentStackOmitted(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("apply", string(data), "", "", 0)
	require.NotNil(t, result)

	asMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, asMap, "component")
	assert.NotContains(t, asMap, "stack")
}

// TestBuildTerraformExecData_DeployParsedAsApply mirrors
// TestParseTerraformResourceChanges_DeployParsedAsApply for the
// single-component combined-object shape.
func TestBuildTerraformExecData_DeployParsedAsApply(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("deploy", string(data), "web", "plat-use2-dev", 0)
	require.NotNil(t, result)
}

// TestBuildTerraformExecData_NonTerraformSubcommand verifies nil is returned
// for a subcommand this shape doesn't cover at all (e.g. "output"), matching
// parseTerraformResourceChanges's existing coverage shape.
func TestBuildTerraformExecData_NonTerraformSubcommand(t *testing.T) {
	assert.Nil(t, buildTerraformExecData("output", "anything", "", "", 0))
}

// TestBuildTerraformExecData_EmptyListsAreNotNull verifies changes/warnings/
// errors always marshal as [], never null, when empty (research.md
// Decision 26) — the regression check for a real CI payload
// (atmos-pro-qa-3 run 32412509172) that showed errors:null/changes:null.
func TestBuildTerraformExecData_EmptyListsAreNotNull(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	// apply_success.txt has resource changes, so use a run with no changes
	// (empty captured output) to exercise the empty-list defaults instead.
	result := buildTerraformExecData("plan", "", "web", "plat-use2-dev", 0)
	require.NotNil(t, result)

	marshaled, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(marshaled, &decoded))
	assert.JSONEq(t, "[]", string(decoded["changes"]))
	assert.JSONEq(t, "[]", string(decoded["warnings"]))
	assert.JSONEq(t, "[]", string(decoded["errors"]))

	// A successfully-parsed run's warnings/errors must also normalize to []
	// rather than surface encoding/json's null for a nil Go slice.
	successResult := buildTerraformExecData("apply", string(data), "", "", 0)
	require.NotNil(t, successResult)
	successMarshaled, err := json.Marshal(successResult)
	require.NoError(t, err)
	var successDecoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(successMarshaled, &successDecoded))
	assert.JSONEq(t, "[]", string(successDecoded["warnings"]))
	assert.JSONEq(t, "[]", string(successDecoded["errors"]))
}

// TestBuildTerraformExecData_UnparseableOutputStillAttachesMinimalData
// verifies a covered subcommand (plan/apply) whose output can't be parsed at
// all still gets a defaulted TerraformExecData with version/exit_code/
// component/stack populated, rather than Data being omitted entirely
// (research.md Decision 29) — exit_code must remain available precisely when
// the rest of the payload is empty.
func TestBuildTerraformExecData_UnparseableOutputStillAttachesMinimalData(t *testing.T) {
	result := buildTerraformExecData("plan", "not terraform output at all", "web", "plat-use2-dev", 2)
	require.NotNil(t, result)

	asMap, ok := result.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, terraformExecDataVersion, asMap["version"])
	assert.Equal(t, 2, asMap["exit_code"])
	assert.Equal(t, "web", asMap["component"])
	assert.Equal(t, "plat-use2-dev", asMap["stack"])
	assert.Equal(t, false, asMap["has_changes"])
	assert.Equal(t, false, asMap["has_errors"])

	resourceCounts, ok := asMap["resource_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 0, resourceCounts["create"])
	assert.Equal(t, 0, resourceCounts["change"])
	assert.Equal(t, 0, resourceCounts["replace"])
	assert.Equal(t, 0, resourceCounts["destroy"])

	assert.Equal(t, map[string]any{}, asMap["outputs"])

	marshaled, err := json.Marshal(result)
	require.NoError(t, err)
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(marshaled, &decoded))
	assert.JSONEq(t, "[]", string(decoded["changes"]))
	assert.JSONEq(t, "[]", string(decoded["warnings"]))
	assert.JSONEq(t, "[]", string(decoded["errors"]))
}

// TestMaskSensitiveOutputs covers FR-010a/research.md Decision 19: a
// Sensitive:true entry's Value is replaced with pkg/io.MaskReplacement while
// Type/Sensitive pass through unchanged; a Sensitive:false entry's Value
// passes through unchanged; a malformed/undecodable entry defaults to masked
// (fail-safe), never forwarding raw, undecoded bytes.
func TestMaskSensitiveOutputs(t *testing.T) {
	outputs := map[string]json.RawMessage{
		"secret_key": json.RawMessage(`{"Value":"top-secret","Type":"string","Sensitive":true}`),
		"bucket_arn": json.RawMessage(`{"Value":"arn:aws:s3:::prod-bucket","Type":"string","Sensitive":false}`),
		"malformed":  json.RawMessage(`not-json`),
	}

	result := maskSensitiveOutputs(outputs)

	secret, ok := result["secret_key"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, iolib.MaskReplacement, secret["value"])
	assert.Equal(t, "string", secret["type"])
	assert.Equal(t, true, secret["sensitive"])

	bucket, ok := result["bucket_arn"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "arn:aws:s3:::prod-bucket", bucket["value"])
	assert.Equal(t, false, bucket["sensitive"])

	malformed, ok := result["malformed"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, iolib.MaskReplacement, malformed["value"])
	assert.Equal(t, true, malformed["sensitive"])
}

// TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser is the regression
// guard for research.md Decision 18's ciMode-decoupling: plan.go/apply.go/
// deploy.go's RunE call terraformCaptureShellOpts unconditionally (no ciMode
// check gates it), so stdout/stderr capture and the exec-metadata parser
// closure must always be wired, regardless of whether the caller separately
// decides to also use ciMode for its own CI-job-summary post-processing.
func TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser(t *testing.T) {
	opts, stdoutBuf, stderrBuf := terraformCaptureShellOpts("web", "plat-use2-dev")

	require.NotNil(t, stdoutBuf)
	require.NotNil(t, stderrBuf)
	require.Len(t, opts, 3, "WithStdoutCapture, WithStderrCapture, WithExecMetadataParser")
}

// TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime verifies the
// closure terraformCaptureShellOpts wires into WithExecMetadataParser reads
// whatever has been written into stdoutBuf/stderrBuf by the time it's
// invoked, combining both streams and stripping ANSI before parsing, and
// threads component/stack through to buildTerraformExecData (research.md
// Decision 21) — mirroring how ExecuteTerraform populates the buffers during
// execution and captureExecMetadataSync invokes the parser afterward.
func TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime(t *testing.T) {
	fixture, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	var stdoutBuf, stderrBuf bytes.Buffer
	parser := terraformExecMetadataParserFunc(&stdoutBuf, &stderrBuf, "web", "plat-use2-dev")

	// Nothing written yet: a covered subcommand ("apply") with unparseable
	// (empty) output still gets a minimal defaulted Data, not nil
	// (research.md Decision 29) — exit_code is threaded through even here.
	emptyResult := parser("apply", 0)
	require.NotNil(t, emptyResult)
	emptyMap, ok := emptyResult.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 0, emptyMap["exit_code"])

	stdoutBuf.WriteString(string(fixture))

	result := parser("apply", 0)
	require.NotNil(t, result)
	asMap, ok := result.(map[string]any)
	require.True(t, ok)
	require.Contains(t, asMap, "resource_counts")
	resourceCounts, ok := asMap["resource_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 3, resourceCounts["create"])
	assert.Equal(t, "web", asMap["component"])
	assert.Equal(t, "plat-use2-dev", asMap["stack"])
}

// TestTerraformNodeHooks_RecordExecResultAccumulates verifies After()
// accumulates one execNodeResult per node call, with the correct
// component/stack/exitCode, and that concurrent calls (as the scheduler may
// dispatch nodes concurrently) are all safely recorded. The two nodes below
// carrying different ExitCode values (0 and 1) in the same aggregate result
// is also the regression guard for research.md Decision 28: exit_code is
// per-component for multi-component runs, never a single aggregate value —
// execNodeResult.ExitCode already provides this, so a future change that
// collapsed it to one shared value would fail this assertion.
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
