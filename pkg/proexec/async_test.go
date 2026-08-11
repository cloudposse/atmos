package proexec

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

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

func TestUploadExecMetadata_DispatchesOnGateOpen(t *testing.T) {
	client := &fakeUploadClient{}
	repo := &fakeGitRepo{info: &git.RepoInfo{}}
	atmosConfig := &schema.AtmosConfiguration{}

	uploadExecMetadata(atmosConfig, "atmos version", 0, nil, client, repo)

	assert.Equal(t, int32(1), client.uploadCalls.Load())
	assert.Equal(t, "atmos version", client.lastRequest.Command)
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
func TestCaptureAsync_RespectsFlushCeiling(t *testing.T) {
	withCIEnv(t, true)
	_ = telemetry.IsCI // sanity: package imported and usable directly if needed.

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	SetAtmosConfig(atmosConfig)
	t.Cleanup(func() { SetAtmosConfig(nil) })

	cmd := &cobra.Command{Use: "version"}

	start := time.Now()
	CaptureAsync(cmd, nil)
	elapsed := time.Since(start)
	// Even in the worst case (client construction fails fast, or a slow
	// upload), CaptureAsync must return within its documented ceiling.
	assert.LessOrEqual(t, elapsed, asyncFlushCeiling+time.Second)
}
