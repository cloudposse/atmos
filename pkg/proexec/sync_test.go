package proexec

import (
	"net/http"
	"net/http/httptest"
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

	err := CaptureSync(atmosConfig, &ExecRecordInput{Command: "describe affected"})
	assert.NoError(t, err)
}

// TestCaptureSync_WarnAndContinueOnFailure verifies a delivery failure (here,
// a Pro endpoint that never responds within the configured sync timeout)
// returns nil (warn-and-continue) rather than propagating the upload error to
// the caller, and that CaptureSync actually exercises its timeout branch
// rather than returning early on a connection failure.
func TestCaptureSync_WarnAndContinueOnFailure(t *testing.T) {
	withCIEnv(t, true)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds = defaultSyncTimeoutSeconds

	start := time.Now()
	err := CaptureSync(atmosConfig, &ExecRecordInput{Command: "terraform apply"})
	elapsed := time.Since(start)

	assert.NoError(t, err, "CaptureSync must warn-and-continue, never return the upload error")
	// The endpoint never responds, so CaptureSync must have hit its timeout
	// branch — not returned early on an immediate connection failure.
	assert.GreaterOrEqual(t, elapsed, defaultSyncTimeoutSeconds*time.Second)
	// Must not hang indefinitely — bounded well beyond the configured timeout.
	assert.Less(t, elapsed, 60*time.Second)
}
