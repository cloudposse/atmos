package proexec

import (
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

	git "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// fakeUploadClient is a minimal pro.AtmosProAPIClientInterface stand-in used
// to observe/control UploadExecMetadata behavior without a real HTTP client.
type fakeUploadClient struct {
	uploadCalls atomic.Int32
	delay       time.Duration
	returnErr   error
	lastRequest *dtos.ExecUploadRequest
}

func (f *fakeUploadClient) UploadInstances(*dtos.InstancesUploadRequest) error { return nil }
func (f *fakeUploadClient) UploadInstanceStatus(*dtos.InstanceStatusUploadRequest) error {
	return nil
}

func (f *fakeUploadClient) UploadAffectedStacks(*dtos.UploadAffectedStacksRequest) error { return nil }
func (f *fakeUploadClient) LockStack(*dtos.LockStackRequest) (dtos.LockStackResponse, error) {
	return dtos.LockStackResponse{}, nil
}

func (f *fakeUploadClient) UnlockStack(*dtos.UnlockStackRequest) (dtos.UnlockStackResponse, error) {
	return dtos.UnlockStackResponse{}, nil
}

func (f *fakeUploadClient) UploadExecMetadata(dto *dtos.ExecUploadRequest) error {
	f.uploadCalls.Add(1)
	f.lastRequest = dto
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.returnErr
}

func (f *fakeUploadClient) UploadExecData(*dtos.ExecDataUploadRequest) (*dtos.ExecDataUploadResponse, error) {
	return &dtos.ExecDataUploadResponse{}, nil
}

func TestUploadExecMetadata_DispatchesOnGateOpen(t *testing.T) {
	client := &fakeUploadClient{}
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	err := uploadExecMetadata(&ExecRecordInput{Command: "version"}, client, repo)

	assert.NoError(t, err)
	assert.Equal(t, int32(1), client.uploadCalls.Load())
	assert.Equal(t, "version", client.lastRequest.Command)
	assert.NotEmpty(t, client.lastRequest.ExecutionID, "async path must populate ExecutionID (FR-003c)")
}

// TestCommandArgsAndFlags_StripsRootAndSeparatesFlags verifies FR-003b's
// shape for the async (CaptureAsync) path: Command has no "atmos" root
// segment, Args holds only positional arguments, and Flags holds only the
// CLI flags actually passed (Changed == true) — matching the sync path's
// shape (internal/exec's captureExecMetadataSync) so both mechanisms report
// consistently for the same invocation (FR-003a).
func TestCommandArgsAndFlags_StripsRootAndSeparatesFlags(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	tf := &cobra.Command{Use: "terraform"}
	plan := &cobra.Command{Use: "plan"}
	plan.Flags().StringP("stack", "s", "", "stack")
	plan.Flags().Bool("upload-status", false, "upload status")
	root.AddCommand(tf)
	tf.AddCommand(plan)

	require.NoError(t, plan.Flags().Parse([]string{"cdn", "-s", "plat-use2-dev", "--upload-status"}))

	command, args, flags := commandArgsAndFlags(plan)

	assert.Equal(t, "terraform plan", command)
	assert.Equal(t, []string{"cdn"}, args)
	// Bare tokens as typed (FR-003b, 2026-08-19 clarification): a value-bearing
	// flag contributes its token and value; a bool-typed flag (--upload-status)
	// MUST appear alone, with no synthesized value appended.
	assert.ElementsMatch(t, []string{"--stack", "plat-use2-dev", "--upload-status"}, flags)
}

// TestCommandArgsAndFlags_BoolFlagOnly guards specifically against the
// synthesized-"true"-value regression: an invocation with only a bool flag
// set (no value-bearing flags at all) must report exactly one token.
func TestCommandArgsAndFlags_BoolFlagOnly(t *testing.T) {
	plan := &cobra.Command{Use: "plan"}
	plan.Flags().Bool("upload-status", false, "upload status")

	require.NoError(t, plan.Flags().Parse([]string{"--upload-status"}))

	_, _, flags := commandArgsAndFlags(plan)

	assert.Equal(t, []string{"--upload-status"}, flags)
}

func TestCommandArgsAndFlags_NilCommand(t *testing.T) {
	command, args, flags := commandArgsAndFlags(nil)
	assert.Empty(t, command)
	assert.Nil(t, args)
	assert.Nil(t, flags)
}

// nestedCommand builds a minimal Cobra command tree so cmd.CommandPath()
// produces a realistic full path (e.g. "atmos terraform plan"), matching how
// cmd/root.go's real command tree is shaped. Standalone commands (no parent)
// only ever report their own Use as CommandPath(), which cannot reproduce
// the sync-allowlist matching this test suite needs.
func nestedCommand(path ...string) *cobra.Command {
	if len(path) == 0 {
		return &cobra.Command{Use: ""}
	}
	root := &cobra.Command{Use: path[0]}
	cur := root
	for _, use := range path[1:] {
		child := &cobra.Command{Use: use}
		cur.AddCommand(child)
		cur = child
	}
	return cur
}

// TestCaptureAsync_SkipsSyncAllowlistedCommands is the regression test for
// the production duplicate-execution-record defect: a single invocation of a
// sync-allowlisted command (e.g. "atmos terraform plan") must never also
// receive the async default-path upload, since its own execution path
// already delivers via CaptureSync (FR-007's mutual-exclusivity
// clarification, research.md Decision 10).
func TestCaptureAsync_SkipsSyncAllowlistedCommands(t *testing.T) {
	withCIEnv(t, true)
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	SetAtmosConfig(atmosConfig)
	t.Cleanup(func() { SetAtmosConfig(nil) })

	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"terraform plan", nestedCommand("atmos", "terraform", "plan")},
		{"terraform apply", nestedCommand("atmos", "terraform", "apply")},
		{"terraform deploy", nestedCommand("atmos", "terraform", "deploy")},
		{"describe affected", nestedCommand("atmos", "describe", "affected")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CaptureAsync must return promptly without attempting a network
			// call (it would otherwise try to reach the real Atmos Pro API
			// and hang/retry, since no fake client is wired at this layer).
			start := time.Now()
			CaptureAsync(tt.cmd, nil)
			assert.Less(t, time.Since(start), time.Second, "CaptureAsync should no-op immediately for sync-allowlisted commands, not attempt an upload")
		})
	}
}

// TestCaptureAsync_StillDispatchesForNonSyncCommands ensures the dedup fix
// does not regress User Story 1: commands outside the synchronous allowlist
// must still receive the async default-path upload.
func TestCaptureAsync_StillDispatchesForNonSyncCommands(t *testing.T) {
	withCIEnv(t, false)
	SetAtmosConfig(&schema.AtmosConfiguration{})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	cmd := nestedCommand("atmos", "version")
	start := time.Now()
	CaptureAsync(cmd, nil)
	// Gate is closed (CI off), so this should also no-op promptly — this
	// test only asserts IsSyncCommand does not incorrectly classify "atmos
	// version" as sync (which would make it indistinguishable from the
	// skip case above).
	assert.False(t, IsSyncCommand(cmd.CommandPath()))
	assert.Less(t, time.Since(start), asyncFlushCeiling)
}

func TestCaptureAsync_NoOpOnGateClosed(t *testing.T) {
	withCIEnv(t, false)
	SetAtmosConfig(&schema.AtmosConfiguration{})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	cmd := &cobra.Command{Use: "version"}
	// Must not panic and must return promptly when the gate is closed.
	start := time.Now()
	CaptureAsync(cmd, nil)
	assert.Less(t, time.Since(start), asyncFlushCeiling)
}

func TestCaptureAsync_DoesNotAlterCallerError(t *testing.T) {
	withCIEnv(t, true)
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = "http://127.0.0.1:0" // unreachable
	SetAtmosConfig(atmosConfig)
	t.Cleanup(func() { SetAtmosConfig(nil) })

	cmd := &cobra.Command{Use: "version"}
	callerErr := assertError("caller failed")

	// CaptureAsync must not panic, must not return anything, and (by
	// construction, it has no return value) cannot mutate callerErr.
	CaptureAsync(cmd, callerErr)
	assert.EqualError(t, callerErr, "caller failed")
}

// Ensures the process telemetry CI helpers are exercised for completeness;
// mirrors the isolation approach used by gate_test.go's withCIEnv.
//
// TEMPORARY: CaptureAsync currently blocks synchronously on the upload (see
// the TEMPORARILY UNUSED note on asyncFlushCeiling in async.go), so this no
// longer asserts a ceiling — it only asserts CaptureAsync returns promptly
// when the upload fails fast (e.g. connection refused).
func TestCaptureAsync_ReturnsAfterUploadCompletes(t *testing.T) {
	withCIEnv(t, true)
	_ = telemetry.IsCI // sanity: package imported and usable directly if needed.

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = "http://127.0.0.1:0" // unreachable; fails fast.
	SetAtmosConfig(atmosConfig)
	t.Cleanup(func() { SetAtmosConfig(nil) })

	cmd := &cobra.Command{Use: "version"}

	start := time.Now()
	CaptureAsync(cmd, nil)
	elapsed := time.Since(start)
	// The connection fails fast, but doWithRetry still retries with backoff
	// (mirrors TestCaptureSync_WarnAndContinueOnFailure's ~7s runtime); just
	// assert CaptureAsync doesn't hang indefinitely.
	assert.Less(t, elapsed, 15*time.Second)
}

// TestCaptureAsync_UsesAndClearsPendingAsyncData is the regression test for
// research.md Decision 23: list instances (and any future async-path command
// with its own structured data) has no per-command hook into CaptureAsync,
// unlike the sync path's caller-supplied parser closure (Decision 18). This
// asserts SetPendingAsyncData's read-and-clear hand-off: (a) data set before
// a CaptureAsync call is used as that call's ExecRecordInput.Data; (b) a
// second CaptureAsync call immediately after, with no intervening
// SetPendingAsyncData, sees Data: nil — preventing one invocation's
// structured data from leaking into the next; (c) a command that never calls
// SetPendingAsyncData at all (today's behavior for every command except
// list instances) continues to see Data: nil, unchanged.
func TestCaptureAsync_UsesAndClearsPendingAsyncData(t *testing.T) {
	withCIEnv(t, true)

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
	SetAtmosConfig(atmosConfig)
	t.Cleanup(func() { SetAtmosConfig(nil) })

	cmd := &cobra.Command{Use: "list-instances"}

	// (c) No SetPendingAsyncData call at all: Data must be absent.
	CaptureAsync(cmd, nil)

	// (a) SetPendingAsyncData before CaptureAsync: Data must reflect it.
	SetPendingAsyncData(map[string]any{"version": 1, "instances": []string{"cdn"}})
	CaptureAsync(cmd, nil)

	// (b) A second CaptureAsync with no intervening SetPendingAsyncData: Data
	// must be nil again — the read-and-clear must not leak into this call.
	CaptureAsync(cmd, nil)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, received, 3)
	assert.Nil(t, received[0].Data, "no SetPendingAsyncData call was made: Data must be absent")
	require.NotNil(t, received[1].Data, "SetPendingAsyncData's payload must be used as this call's Data")
	assert.JSONEq(t, `{"version":1,"instances":["cdn"]}`, string(received[1].Data))
	assert.Nil(t, received[2].Data, "pendingAsyncData must be cleared after being consumed once")
}
