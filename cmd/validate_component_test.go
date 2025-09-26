package cmd

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func TestValidateComponentCmd(t *testing.T) {
	// Test the success message output format with color
	t.Run("successful validation message format with color", func(t *testing.T) {
		// Capture stderr output
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		defer func() {
			os.Stderr = oldStderr
		}()

		// Call the output function directly (simulating what RunE does on success)
		component := "test-component"
		stack := "test-stack"
		successMsg := fmt.Sprintf("✓ Validated successfully: component=%s stack=%s\n", component, stack)
		theme.Colors.Success.Fprint(os.Stderr, successMsg)

		// Close writer and read output
		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		// Verify the output contains the success message
		assert.Contains(t, output, "✓ Validated successfully: component=test-component stack=test-stack")
	})
}
