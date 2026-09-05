package exec

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCaptureExecMetadataSync_FlagsReflectRealInvocation is a regression test
// for a production bug: an `atmos terraform plan cdn -s plat-use2-dev
// --upload-status` invocation uploaded an execution record whose `flags`
// field was empty, even though real flags were passed on the command line.
//
// Root cause: captureExecMetadataSync (internal/exec/terraform.go) used to
// report info.AdditionalArgsAndFlags as the record's Flags. That field is NOT
// "the CLI flags the user passed to atmos" — per cli_utils.go, it holds only
// the pass-through args left over after Cobra has already parsed and
// consumed recognized atmos flags like `-s`/`--stack`. So `-s plat-use2-dev`
// was never in that slice to begin with, and the one atmos-recognized flag
// that IS threaded through it, `--upload-status`, is explicitly stripped out
// in buildPlanSubcommandArgs (terraform_execute_helpers_args.go) before
// capture runs. The fix: Flags is now sourced from the invoking
// *cobra.Command's own record of explicitly-set flags
// (proexec.FlagsFromCommand), matching the async path.
func TestCaptureExecMetadataSync_FlagsReflectRealInvocation(t *testing.T) {
	t.Setenv("CI", "true")

	var mu sync.Mutex
	var received []dtos.ExecUploadRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/atmos/exec") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req dtos.ExecUploadRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		mu.Lock()
		received = append(received, req)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Token = "test-token"
	// A short-lived Pro exec sync timeout keeps this test fast if the upload
	// path ever regresses to hanging instead of completing.
	atmosConfig.Settings.Pro.Exec.SyncTimeout = 2 * time.Second

	// Mirrors the real invocation `atmos terraform plan cdn -s plat-use2-dev
	// --upload-status`: the planCmd's own flags are set exactly as the user
	// would type them, and AdditionalArgsAndFlags is empty (as it would be
	// after buildPlanSubcommandArgs strips --upload-status), proving Flags no
	// longer depends on that field at all.
	plan := &cobra.Command{Use: "plan"}
	plan.Flags().StringP("stack", "s", "", "stack")
	plan.Flags().Bool("upload-status", false, "upload status")
	require.NoError(t, plan.Flags().Parse([]string{"-s", "plat-use2-dev", "--upload-status"}))

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:       "cdn",
		Stack:                  "plat-use2-dev",
		AdditionalArgsAndFlags: []string{},
	}

	captureExecMetadataSync(atmosConfig, "plan", info, execMetadataSyncParams{Cmd: plan})

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, received, 1, "expected exactly one exec-metadata upload for a single invocation")
	assert.ElementsMatch(t, []string{"--stack", "plat-use2-dev", "--upload-status"}, received[0].Flags,
		"execution record's Flags must reflect the real CLI flags used, as bare tokens with no synthesized value for the bool flag")
}

// fakeNodeHooks is a minimal schema.ComponentNodeHooks implementation used
// only to give info.NodeHooks a non-nil value — captureExecMetadataSync only
// checks it for nilness, it never calls through the interface.
type fakeNodeHooks struct{}

func (fakeNodeHooks) Before(_ context.Context, _ *schema.ConfigAndStacksInfo) error { return nil }

func (fakeNodeHooks) After(_ context.Context, _ *schema.ConfigAndStacksInfo, _ string, _ error) error {
	return nil
}

// TestCaptureExecMetadataSync_SkipsPerNodeWhenNodeHooksWired is the
// regression test for FR-006a: a multi-component graph run must never
// produce one execution record per node — cmd/terraform/utils.go's
// terraformNodeHooks aggregates and fires a single record after the whole
// run completes instead (captureMultiComponentExecMetadata). This is
// signaled to captureExecMetadataSync via info.NodeHooks being non-nil,
// exactly as cmd/terraform/utils.go's wirePerComponentHook sets it for
// --affected/--all/query multi-component runs.
func TestCaptureExecMetadataSync_SkipsPerNodeWhenNodeHooksWired(t *testing.T) {
	t.Setenv("CI", "true")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.Exec.SyncTimeout = 2 * time.Second

	plan := &cobra.Command{Use: "plan"}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "cdn",
		Stack:            "plat-use2-dev",
		NodeHooks:        fakeNodeHooks{},
	}

	captureExecMetadataSync(atmosConfig, "plan", info, execMetadataSyncParams{Cmd: plan})

	assert.Equal(t, int32(0), requestCount.Load(),
		"a multi-component graph node must not fire its own exec-metadata upload — the aggregate call after the whole run completes owns delivery")
}

// TestCaptureExecMetadataSync_CallsParserForSyncAllowlistedSingleComponent is
// the regression test for research.md Decision 18: for a single-component
// sync-allowlisted invocation (info.NodeHooks == nil), captureExecMetadataSync
// MUST call the supplied parser closure exactly once and forward its return
// value as the uploaded record's Data.
func TestCaptureExecMetadataSync_CallsParserForSyncAllowlistedSingleComponent(t *testing.T) {
	t.Setenv("CI", "true")

	var received dtos.ExecUploadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.Exec.SyncTimeout = 2 * time.Second

	plan := &cobra.Command{Use: "plan"}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "cdn", Stack: "plat-use2-dev"}

	var calls atomic.Int32
	var gotSubCommand string
	var gotExitCode int
	parser := func(subCommand string, exitCode int, output string) any {
		calls.Add(1)
		gotSubCommand = subCommand
		gotExitCode = exitCode
		return map[string]any{"resource_counts": map[string]any{"create": 1}}
	}

	captureExecMetadataSync(atmosConfig, "plan", info, execMetadataSyncParams{
		Cmd: plan, Parser: parser, Err: errUtils.ExitCodeError{Code: 2},
	})

	assert.Equal(t, int32(1), calls.Load(), "parser must be called exactly once")
	assert.Equal(t, "plan", gotSubCommand)
	assert.Equal(t, 2, gotExitCode, "parser must receive the same exit code derived from params.Err (research.md Decision 27)")
	require.NotNil(t, received.Data)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(received.Data, &decoded))
	assert.Contains(t, decoded, "resource_counts")
}

// TestCaptureExecMetadataSync_NeverCallsParserForNonSyncSubcommand verifies
// the parser closure is never invoked for a subcommand outside the
// synchronous exec-metadata allowlist.
func TestCaptureExecMetadataSync_NeverCallsParserForNonSyncSubcommand(t *testing.T) {
	t.Setenv("CI", "")

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	var calls atomic.Int32
	parser := func(string, int, string) any {
		calls.Add(1)
		return "should never be reached"
	}

	captureExecMetadataSync(atmosConfig, "validate", info, execMetadataSyncParams{Parser: parser})

	assert.Equal(t, int32(0), calls.Load())
}

// TestCaptureExecMetadataSync_NeverCallsParserWhenNodeHooksWired verifies the
// parser closure is never invoked for a multi-component node
// (info.NodeHooks != nil) — that path's structured data is folded in by
// cmd/terraform/utils.go's aggregator instead (research.md Decision 17).
func TestCaptureExecMetadataSync_NeverCallsParserWhenNodeHooksWired(t *testing.T) {
	t.Setenv("CI", "")

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{NodeHooks: fakeNodeHooks{}}

	var calls atomic.Int32
	parser := func(string, int, string) any {
		calls.Add(1)
		return "should never be reached"
	}

	captureExecMetadataSync(atmosConfig, "plan", info, execMetadataSyncParams{Parser: parser})

	assert.Equal(t, int32(0), calls.Load())
}
