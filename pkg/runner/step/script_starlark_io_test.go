package step

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkOutputUsesMaskedIOChannels(t *testing.T) {
	// The shared I/O setup changes process globals, so these cases are sequential.
	for _, color := range []string{"0", "1"} {
		for _, mode := range []OutputMode{OutputModeRaw, OutputModeLog, OutputModeNone} {
			t.Run(fmt.Sprintf("color=%s/%s", color, mode), func(t *testing.T) {
				t.Setenv("NO_COLOR", "")
				t.Setenv("ATMOS_FORCE_COLOR", color)
				stdout, stderr, cleanup := setupOutputModeCapture(t)
				defer cleanup()
				iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
				secret := "starlark-private-value-72c104"
				iolib.GetContext().Masker().RegisterValue(secret)
				masked := iolib.MaskString(secret)
				require.NotEqual(t, secret, masked)
				result, err := (&ScriptHandler{}).Execute(context.Background(), &schema.WorkflowStep{
					Name: "channels", Interpreter: "starlark", Output: string(mode),
					Show: &schema.ShowConfig{Labels: BoolPtr(false)}, Env: map[string]string{"SECRET": secret},
					Script: `
def branch():
    print("data-channel: " + env["SECRET"])
    ui.info("ui-channel: " + env["SECRET"])
steps.parallel(functions=[branch, branch])
output = env["SECRET"]
`,
				}, NewVariables())
				require.NoError(t, err)
				cleanup()
				// Captured values are usable by scripts; terminal writes are masked.
				assert.Contains(t, result.Value, secret)
				assert.Contains(t, result.Metadata["stdout"], secret)
				assert.NotContains(t, stdout.String(), secret)
				assert.NotContains(t, stderr.String(), secret)
				if mode == OutputModeNone {
					assert.Empty(t, stdout.String())
					assert.Empty(t, stderr.String())
					return
				}
				assert.Contains(t, stdout.String(), "data-channel: "+masked)
				assert.NotContains(t, stdout.String(), "ui-channel:")
				assert.Contains(t, ansi.Strip(stderr.String()), "ui-channel: "+masked)
				assert.NotContains(t, stderr.String(), "data-channel:")
			})
		}
	}
}
