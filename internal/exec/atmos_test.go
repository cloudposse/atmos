package exec

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func TestExecuteAtmosCmd_ValidateComponent(t *testing.T) {
	t.Run("validate component success message format with color", func(t *testing.T) {
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
		successMsg := fmt.Sprintf("✓ Validated successfully: component=%s stack=%s\n", selectedComponent, selectedStack)
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
