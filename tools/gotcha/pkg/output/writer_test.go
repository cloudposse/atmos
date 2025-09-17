package output

import (
	"bytes"
	"os"
	"testing"

	"github.com/cloudposse/gotcha/pkg/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name        string
		setupEnv    func()
		cleanupEnv  func()
		wantUnified bool
		description string
	}{
		{
			name: "standard terminal mode",
			setupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				os.Unsetenv("GOTCHA_SPLIT_STREAMS")
			},
			cleanupEnv:  func() {},
			wantUnified: false,
			description: "Should use split streams in terminal",
		},
		{
			name: "GitHub Actions mode",
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Unsetenv("GOTCHA_SPLIT_STREAMS")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			wantUnified: true,
			description: "Should unify streams in GitHub Actions",
		},
		{
			name: "GitHub Actions with split streams override",
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Setenv("GOTCHA_SPLIT_STREAMS", "1")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				os.Unsetenv("GOTCHA_SPLIT_STREAMS")
			},
			wantUnified: false,
			description: "Should respect GOTCHA_SPLIT_STREAMS override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			// Initialize config environment bindings for test
			config.InitEnvironment()

			w := New()
			assert.Equal(t, tt.wantUnified, w.IsUnified(), tt.description)

			// Verify streams are set
			assert.NotNil(t, w.Data)
			assert.NotNil(t, w.UI)
		})
	}
}

func TestNewCustom(t *testing.T) {
	var dataBuf, uiBuf bytes.Buffer

	w := NewCustom(&dataBuf, &uiBuf)

	assert.Equal(t, &dataBuf, w.Data)
	assert.Equal(t, &uiBuf, w.UI)
	assert.False(t, w.IsUnified())
}

func TestWriter_PrintMethods(t *testing.T) {
	var dataBuf, uiBuf bytes.Buffer
	w := NewCustom(&dataBuf, &uiBuf)

	// Test PrintUI
	w.PrintUI("UI message: %s\n", "test")
	assert.Equal(t, "UI message: test\n", uiBuf.String())
	assert.Empty(t, dataBuf.String())

	// Reset buffers
	uiBuf.Reset()
	dataBuf.Reset()

	// Test PrintData
	w.PrintData("Data output: %d\n", 42)
	assert.Equal(t, "Data output: 42\n", dataBuf.String())
	assert.Empty(t, uiBuf.String())
}

func TestWriter_FprintMethods(t *testing.T) {
	var dataBuf, uiBuf bytes.Buffer
	w := NewCustom(&dataBuf, &uiBuf)

	// Test FprintUI
	n, err := w.FprintUI("UI: %s", "test")
	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	assert.Equal(t, "UI: test", uiBuf.String())

	// Test FprintData
	n, err = w.FprintData("Data: %d", 123)
	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	assert.Equal(t, "Data: 123", dataBuf.String())
}

func TestWriter_ConfigureCommand(t *testing.T) {
	var dataBuf, uiBuf bytes.Buffer
	w := NewCustom(&dataBuf, &uiBuf)

	cmd := &cobra.Command{}
	w.ConfigureCommand(cmd)

	// Cobra's SetOut and SetErr don't expose getters,
	// so we test by using the command's output methods
	cmd.Println("test output")
	assert.Contains(t, dataBuf.String(), "test")
}

func TestWriter_UnifiedOutput(t *testing.T) {
	var unifiedBuf bytes.Buffer

	// Create a unified writer (both streams go to same buffer)
	w := &Writer{
		Data:         &unifiedBuf,
		UI:           &unifiedBuf,
		forceUnified: true,
	}

	assert.True(t, w.IsUnified())

	// Both should write to the same buffer
	w.PrintUI("UI message\n")
	w.PrintData("Data message\n")

	output := unifiedBuf.String()
	assert.Contains(t, output, "UI message")
	assert.Contains(t, output, "Data message")
	assert.Equal(t, "UI message\nData message\n", output)
}
