package proexec

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSyncTimeout(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expected    time.Duration
	}{
		{
			name: "below default clamps to default",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Pro: schema.ProSettings{
						Exec: schema.ExecSettings{SyncTimeoutSeconds: 3},
					},
				},
			},
			expected: defaultSyncTimeoutSeconds * time.Second,
		},
		{
			name: "above default is honored",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Pro: schema.ProSettings{
						Exec: schema.ExecSettings{SyncTimeoutSeconds: 30},
					},
				},
			},
			expected: 30 * time.Second,
		},
		{
			name:        "zero uses default",
			atmosConfig: &schema.AtmosConfiguration{},
			expected:    defaultSyncTimeoutSeconds * time.Second,
		},
		{
			name:        "nil config uses default",
			atmosConfig: nil,
			expected:    defaultSyncTimeoutSeconds * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, syncTimeout(tt.atmosConfig))
		})
	}
}

func TestCaptureSync_NoOpOnGateClosed(t *testing.T) {
	withCIEnv(t, false)
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"

	err := CaptureSync(atmosConfig, &ExecRecordInput{Command: "describe affected"})
	assert.NoError(t, err)
}

// TestCaptureSync_DeliversSuccessfully verifies a successful upload: the
// request reaches the Pro endpoint with the expected method and payload, and
// CaptureSync returns before the configured sync timeout elapses.
func TestCaptureSync_DeliversSuccessfully(t *testing.T) {
	withCIEnv(t, true)

	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = server.URL
	// Deliberately longer than the delivery time, so CaptureSync must return
	// once the delivery completes rather than waiting out the full timeout.
	atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds = defaultSyncTimeoutSeconds * 10

	start := time.Now()
	err := CaptureSync(atmosConfig, &ExecRecordInput{Command: "terraform apply"})
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Contains(t, gotPath, "/atmos/exec")
	assert.Equal(t, "terraform apply", gotBody["command"])
	assert.Less(t, elapsed, defaultSyncTimeoutSeconds*time.Second)
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
