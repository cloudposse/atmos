package terraform

// utils_exec_metadata_test.go covers the multi-component exec-metadata
// aggregation added for FR-006a (research.md Decisions 11/17): a
// multi-component graph run must produce exactly one execution record for
// the whole invocation, with each node's identity/outcome folded into that
// single record's Data field, not sent as separate per-node records.

import (
	"context"
	"encoding/base64"
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

	logs, ok := asMap["logs"].(string)
	require.True(t, ok)
	decoded, err := base64.StdEncoding.DecodeString(logs)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(decoded))
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

	logs, ok := asMap["logs"].(string)
	require.True(t, ok)
	decodedLogs, err := base64.StdEncoding.DecodeString(logs)
	require.NoError(t, err)
	assert.Equal(t, "not terraform output at all", string(decodedLogs))

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

// TestBuildTerraformExecData_SensitiveOutputNeverUploadedInAnyForm is the
// end-to-end regression guard for the actual safety property that matters:
// no real secret value ever reaches the exec-metadata Data payload for a
// Terraform-sensitive output, regardless of whether the upstream parser
// correctly flags it Sensitive (see the known-limitation note on
// pkg/ci/plugins/terraform's extractApplyOutputs — the regex console parser
// never sets Sensitive: true, because Terraform's own console text already
// replaces a sensitive output's real value with "<sensitive>" before it's
// ever captured). Even with that inaccurate flag, this test proves the
// value attached to Data.outputs is Terraform's own safe placeholder text,
// never a real secret — "we should not upload sensitive data in any case"
// holds in practice.
func TestBuildTerraformExecData_SensitiveOutputNeverUploadedInAnyForm(t *testing.T) {
	output := `aws_instance.web: Creating...
aws_instance.web: Creation complete after 35s [id=i-12345678]

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.

Outputs:

instance_id = "i-12345678"
secret_key = <sensitive>
`

	result := buildTerraformExecData("apply", output, "web", "plat-use2-dev", 0)
	require.NotNil(t, result)
	asMap, ok := result.(map[string]any)
	require.True(t, ok)

	outputs, ok := asMap["outputs"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, outputs, "secret_key")
	secret, ok := outputs["secret_key"].(map[string]any)
	require.True(t, ok)
	// Known limitation: sensitive stays false (see extractApplyOutputs'
	// doc comment) — but the value itself is still never a real secret.
	assert.Equal(t, false, secret["sensitive"])
	assert.Equal(t, "<sensitive>", secret["value"], "must be Terraform's own placeholder, never a real secret value")

	logs, ok := asMap["logs"].(string)
	require.True(t, ok)
	decodedLogs, err := base64.StdEncoding.DecodeString(logs)
	require.NoError(t, err)
	assert.Contains(t, string(decodedLogs), "<sensitive>", "raw log text must also carry only Terraform's own placeholder")
}

// TestRedactSensitiveOutputsFromRawOutput covers FR-010a's extension to the
// logs field (2026-08-21 clarification): a string-valued, Sensitive:true
// output's literal value is redacted everywhere it appears in the text, a
// Sensitive:false output's value is left untouched, and a non-string
// Sensitive:true value (no single unambiguous literal-text form) is skipped
// rather than attempting a partial/incorrect replacement.
func TestRedactSensitiveOutputsFromRawOutput(t *testing.T) {
	outputs := map[string]json.RawMessage{
		"secret_key": json.RawMessage(`{"Value":"hunter2","Type":"string","Sensitive":true}`),
		"bucket_arn": json.RawMessage(`{"Value":"arn:aws:s3:::prod-bucket","Type":"string","Sensitive":false}`),
		"secret_list": json.RawMessage(`{"Value":["a","b"],"Type":"list","Sensitive":true}`),
	}

	text := `+ password = "hunter2"
+ bucket    = "arn:aws:s3:::prod-bucket"`

	result := redactSensitiveOutputsFromRawOutput(text, outputs)

	assert.NotContains(t, result, "hunter2")
	assert.Contains(t, result, iolib.MaskReplacement)
	assert.Contains(t, result, "arn:aws:s3:::prod-bucket", "non-sensitive value must pass through unchanged")
}

// TestBuildTerraformExecData_LogsWiredThroughRedactionAndEncoding is the
// end-to-end regression guard confirming buildTerraformExecData's logs field
// is populated via redactSensitiveOutputsFromRawOutput then encodeLogs (not
// the bare output string) on the parse-succeeded path. This fixture has no
// Sensitive:true outputs (citerraform's regex-based extractApplyOutputs
// never marks any output Sensitive — see the note on
// redactSensitiveOutputsFromRawOutput), so this only proves non-sensitive
// content survives redaction+encoding round-trip unchanged;
// TestRedactSensitiveOutputsFromRawOutput covers the actual redaction
// behavior directly against the helper.
func TestBuildTerraformExecData_LogsWiredThroughRedactionAndEncoding(t *testing.T) {
	data, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	result := buildTerraformExecData("apply", string(data), "web", "plat-use2-dev", 0)
	require.NotNil(t, result)
	asMap, ok := result.(map[string]any)
	require.True(t, ok)

	logs, ok := asMap["logs"].(string)
	require.True(t, ok)
	decodedLogs, err := base64.StdEncoding.DecodeString(logs)
	require.NoError(t, err)
	assert.Contains(t, string(decodedLogs), "i-12345678", "non-sensitive output value must remain in logs")
}

// TestEncodeLogs verifies encodeLogs base64-encodes its (already-masked)
// input, and that it is masking-then-encoding, not encoding raw text — a
// secret pattern present in the plaintext must not survive as a
// pattern-matchable substring once decoded, since a downstream Gitleaks pass
// over the whole marshaled Data blob cannot see into base64-encoded bytes
// (2026-08-21 clarification: masking must happen before encoding, here, not
// rely on that later pass for this field).
func TestEncodeLogs(t *testing.T) {
	encoded := encodeLogs("hello world")

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(decoded))
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

// TestTerraformExecMetadataParserFunc_UsesSuppliedOutput verifies the closure
// terraformCaptureShellOpts wires into WithExecMetadataParser parses whatever
// output string the caller supplies at invocation time (FR-006f, research.md
// Decision 32 — internal/exec's executeCommandPipeline captures this scoped
// to only the main plan/apply/deploy subprocess, not read from a buffer this
// closure owns), stripping ANSI before parsing, threads component/stack
// through to buildTerraformExecData (research.md Decision 21), and wraps the
// single result in the unified {"version": ..., "components": [...]} shape
// (Decision 38, spec.md 2026-08-21 clarification — single-component Data is
// never structurally different from multi-component, just a one-element
// components list) with its own per-entry "version" stripped.
func TestTerraformExecMetadataParserFunc_UsesSuppliedOutput(t *testing.T) {
	fixture, err := os.ReadFile("../../pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt")
	require.NoError(t, err)

	parser := terraformExecMetadataParserFunc("web", "plat-use2-dev")

	// Empty output: a covered subcommand ("apply") with unparseable (empty)
	// output still gets a minimal defaulted Data, not nil (research.md
	// Decision 29) — exit_code is threaded through even here.
	emptyResult := parser("apply", 0, "")
	require.NotNil(t, emptyResult)
	emptyWrapper, ok := emptyResult.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, terraformExecDataVersion, emptyWrapper["version"])
	emptyComponents, ok := emptyWrapper["components"].([]any)
	require.True(t, ok)
	require.Len(t, emptyComponents, 1)
	emptyMap, ok := emptyComponents[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 0, emptyMap["exit_code"])
	assert.NotContains(t, emptyMap, "version")

	result := parser("apply", 0, string(fixture))
	require.NotNil(t, result)
	wrapper, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, terraformExecDataVersion, wrapper["version"])
	components, ok := wrapper["components"].([]any)
	require.True(t, ok)
	require.Len(t, components, 1)
	asMap, ok := components[0].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, asMap, "version")
	require.Contains(t, asMap, "resource_counts")
	resourceCounts, ok := asMap["resource_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 3, resourceCounts["create"])
	assert.Equal(t, "web", asMap["component"])
	assert.Equal(t, "plat-use2-dev", asMap["stack"])
}

// TestTerraformNodeHooks_RecordExecResultAccumulates verifies After()
// accumulates one execNodeResult per node call, with the correct
// component's own full TerraformExecData entry (FR-006a's restructured
// {"components": [...]} shape, spec.md Session 2026-08-21), and that
// concurrent calls (as the scheduler may dispatch nodes concurrently) are
// all safely recorded. The two nodes below carrying different exit_code
// values (0 and 1) in the same aggregate result is also the regression guard
// for research.md Decision 28: exit_code is per-component for multi-component
// runs, never a single aggregate value.
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
	results := append([]any(nil), nodeHooks.results...)
	nodeHooks.mu.Unlock()

	// One full TerraformExecData entry per node — the second node's is a
	// minimal/defaulted entry since it had no output to parse.
	require.Len(t, results, 2, "one full TerraformExecData entry per node")

	node1, ok := results[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "myapp", node1["component"])
	assert.Equal(t, "dev", node1["stack"])
	assert.Equal(t, 0, node1["exit_code"])
	assert.NotContains(t, node1, "version", "per-component entries omit version — redundant with the outer components wrapper's own version")
	resourceCounts1, ok := node1["resource_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 3, resourceCounts1["create"])

	node2, ok := results[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "myapp", node2["component"])
	assert.Equal(t, "dev", node2["stack"])
	assert.Equal(t, 1, node2["exit_code"])
	assert.NotContains(t, node2, "version")
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
// nodes' full TerraformExecData entries folded into that single request's
// Data.components array (spec.md Session 2026-08-21 restructure) — never one
// request per component.
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

	var data struct {
		Version    int              `json:"version"`
		Components []map[string]any `json:"components"`
	}
	require.NoError(t, json.Unmarshal(receivedBody.Data, &data))
	assert.Equal(t, terraformExecDataVersion, data.Version)
	require.Len(t, data.Components, 2, "both nodes' full TerraformExecData entries must be folded into the single record's Data.components")

	var components []string
	for _, c := range data.Components {
		components = append(components, c["component"].(string))
		assert.Equal(t, "dev", c["stack"])
		assert.Equal(t, float64(0), c["exit_code"])
		assert.NotContains(t, c, "version", "per-component entries omit version — redundant with the outer components wrapper's own version")
	}
	assert.Contains(t, components, "myapp")
	assert.Contains(t, components, "other")
}
