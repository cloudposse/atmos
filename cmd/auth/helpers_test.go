package auth

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "expired (negative duration)",
			duration: -1 * time.Hour,
			expected: "expired",
		},
		{
			name:     "zero seconds",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "one minute",
			duration: 1 * time.Minute,
			expected: "1m 0s",
		},
		{
			name:     "minutes and seconds",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m 30s",
		},
		{
			name:     "one hour",
			duration: 1 * time.Hour,
			expected: "1h 0m",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 15*time.Minute,
			expected: "2h 15m",
		},
		{
			name:     "large duration",
			duration: 48*time.Hour + 30*time.Minute,
			expected: "48h 30m",
		},
		{
			name:     "just under one minute",
			duration: 59 * time.Second,
			expected: "59s",
		},
		{
			name:     "just under one hour",
			duration: 59*time.Minute + 59*time.Second,
			expected: "59m 59s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDisplayAuthSuccess(t *testing.T) {
	// Capture stderr output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create test whoami info.
	expiration := time.Now().Add(1 * time.Hour)
	whoami := &authTypes.WhoamiInfo{
		Provider:   "aws-sso",
		Identity:   "prod-admin",
		Account:    "123456789012",
		Region:     "us-east-1",
		Expiration: &expiration,
	}

	displayAuthSuccess(whoami)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected information.
	assert.Contains(t, output, "Authentication successful")
	assert.Contains(t, output, "Provider")
	assert.Contains(t, output, "aws-sso")
	assert.Contains(t, output, "Identity")
	assert.Contains(t, output, "prod-admin")
	assert.Contains(t, output, "Account")
	assert.Contains(t, output, "123456789012")
	assert.Contains(t, output, "Region")
	assert.Contains(t, output, "us-east-1")
	assert.Contains(t, output, "Expires")
}

func TestDisplayAuthSuccess_MinimalInfo(t *testing.T) {
	// Capture stderr output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create minimal whoami info (no optional fields).
	whoami := &authTypes.WhoamiInfo{
		Provider: "azure",
		Identity: "dev",
	}

	displayAuthSuccess(whoami)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains required info but not optional fields.
	assert.Contains(t, output, "Authentication successful")
	assert.Contains(t, output, "Provider")
	assert.Contains(t, output, "azure")
	assert.Contains(t, output, "Identity")
	assert.Contains(t, output, "dev")
	// Optional fields should not appear.
	assert.NotContains(t, output, "Account")
	assert.NotContains(t, output, "Region")
	assert.NotContains(t, output, "Expires")
}

func TestBuildConfigAndStacksInfo(t *testing.T) {
	// Create a test command.
	cmd := &cobra.Command{
		Use: "test",
	}

	// Create a fresh viper instance.
	v := viper.New()

	// Call the function.
	result := BuildConfigAndStacksInfo(cmd, v)

	// Verify it returns a valid ConfigAndStacksInfo struct.
	// The struct should be empty since we haven't set any flags.
	assert.NotNil(t, result)
}

func TestCreateAuthManager(t *testing.T) {
	// Test with nil auth config - should return error or handle gracefully.
	// This tests that the function doesn't panic with minimal input.
	t.Run("with empty auth config", func(t *testing.T) {
		// Note: CreateAuthManager requires valid config to work.
		// We're testing that it handles the call without panicking.
		// A nil config will likely return an error.
		manager, err := CreateAuthManager(nil)
		// With nil config, we expect an error.
		assert.Error(t, err)
		assert.Nil(t, manager)
	})
}

func TestHandleHelpRequest(t *testing.T) {
	// Note: handleHelpRequest calls os.Exit(0) when help is requested,
	// so we can only test the case where help is NOT requested.
	t.Run("no help flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		args := []string{"some", "args"}

		// Should not panic or exit.
		assert.NotPanics(t, func() {
			handleHelpRequest(cmd, args)
		})
	})

	t.Run("empty args", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		args := []string{}

		// Should not panic or exit.
		assert.NotPanics(t, func() {
			handleHelpRequest(cmd, args)
		})
	})
}
