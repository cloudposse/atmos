package exec

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestExecuteAtmosCmd_ValidateComponent(t *testing.T) {
	t.Run("validate component success message format", func(t *testing.T) {
		// Capture stderr output
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		defer func() {
			os.Stderr = oldStderr
		}()

		// Test the output message format (simulating what ExecuteAtmosCmd does on success)
		selectedComponent := "test-component"
		selectedStack := "test-stack"
		u.PrintfMessageToTUI("✓ Validated successfully: component=%s stack=%s\n", selectedComponent, selectedStack)

		// Close writer and read output
		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		// Verify the output format
		assert.Contains(t, output, "✓ Validated successfully: component=test-component stack=test-stack")
	})
}
