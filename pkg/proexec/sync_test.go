package proexec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSyncTimeout_ClampsBelowDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds = 3

	assert.Equal(t, defaultSyncTimeoutSeconds*time.Second, syncTimeout(atmosConfig))
}

func TestSyncTimeout_HonorsAboveDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds = 30

	assert.Equal(t, 30*time.Second, syncTimeout(atmosConfig))
}

func TestSyncTimeout_ZeroUsesDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	assert.Equal(t, defaultSyncTimeoutSeconds*time.Second, syncTimeout(atmosConfig))
}

func TestSyncTimeout_NilConfigUsesDefault(t *testing.T) {
	assert.Equal(t, defaultSyncTimeoutSeconds*time.Second, syncTimeout(nil))
}

func TestCaptureSync_NoOpOnGateClosed(t *testing.T) {
	withCIEnv(t, false)
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"

	err := CaptureSync(atmosConfig, "describe affected", 0, nil)
	assert.NoError(t, err)
}

// TestCaptureSync_WarnAndContinueOnFailure verifies a delivery failure (here,
// an unreachable Pro endpoint) returns nil (warn-and-continue) rather than
// propagating the upload error to the caller.
func TestCaptureSync_WarnAndContinueOnFailure(t *testing.T) {
	withCIEnv(t, true)
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = "http://127.0.0.1:0"
	atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds = defaultSyncTimeoutSeconds

	start := time.Now()
	err := CaptureSync(atmosConfig, "terraform apply", 0, nil)
	elapsed := time.Since(start)

	assert.NoError(t, err, "CaptureSync must warn-and-continue, never return the upload error")
	// Must not hang indefinitely — bounded by the (clamped) sync timeout.
	assert.Less(t, elapsed, 60*time.Second)
}
