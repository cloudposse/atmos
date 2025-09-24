package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateComponentCmd(t *testing.T) {
	// Test the success message output format
	t.Run("successful validation message format", func(t *testing.T) {
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
		u.PrintfMessageToTUI("✓ Validated successfully: component=%s stack=%s\n", component, stack)

		// Close writer and read output
		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		// Verify the output format
		assert.Contains(t, output, "✓ Validated successfully: component=test-component stack=test-stack")
	})
}
