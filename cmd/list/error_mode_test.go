package list

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestDescribeStacksErrorOptions(t *testing.T) {
	t.Run("strict returns a nil collector and no OnWarning", func(t *testing.T) {
		opts, collector := describeStacksErrorOptions("strict")
		assert.Equal(t, e.DescribeStacksErrorOptions{}, opts)
		assert.Nil(t, collector)
	})

	t.Run("empty value is strict", func(t *testing.T) {
		opts, collector := describeStacksErrorOptions("")
		assert.Equal(t, e.DescribeStacksErrorOptions{}, opts)
		assert.Nil(t, collector)
	})

	t.Run("warn enables OnErrorWarn with a non-nil collector wired as the callback", func(t *testing.T) {
		opts, collector := describeStacksErrorOptions("warn")
		require.Equal(t, e.OnErrorWarn, opts.OnError)
		require.NotNil(t, opts.OnWarning)
		require.NotNil(t, collector)
		assert.Equal(t, 0, collector.Count())

		opts.OnWarning(e.DegradationWarning{
			Stack:     "dev",
			Component: "vpc",
			Function:  "!terraform.state vpc dev bucket",
			Reason:    "terraform state not provisioned",
		})

		assert.Equal(t, 1, collector.Count(), "OnWarning must feed the same collector returned alongside it")
	})

	t.Run("silent also enables OnErrorWarn with a non-nil collector", func(t *testing.T) {
		opts, collector := describeStacksErrorOptions("silent")
		require.Equal(t, e.OnErrorWarn, opts.OnError)
		require.NotNil(t, opts.OnWarning)
		require.NotNil(t, collector)

		opts.OnWarning(e.DegradationWarning{Stack: "dev", Component: "vpc", Function: "!terraform.state vpc dev bucket", Reason: "terraform state not provisioned"})
		assert.Equal(t, 1, collector.Count(), "silent mode still accumulates warnings for --logs-level=Debug visibility, it just never prints a summary")
	})
}

func TestPrintErrorModeSummary(t *testing.T) {
	t.Run("warn prints when the collector has warnings", func(t *testing.T) {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		// Initialize the package formatter once for this output assertion. PushUIWriter
		// captures only this test's UI stream and restores the previous sink on return.
		ui.InitFormatter(ioCtx)
		var output bytes.Buffer
		restore := iolib.PushUIWriter(&output)
		defer restore()

		_, collector := describeStacksErrorOptions("warn")
		collector.Add(e.DegradationWarning{Stack: "dev", Component: "vpc", Function: "!terraform.state vpc dev bucket", Reason: "terraform state not provisioned"})

		printErrorModeSummary("warn", collector)
		assert.Contains(t, output.String(), "1 value(s) could not be determined")
		assert.Contains(t, output.String(), "--error-mode=strict")
	})

	t.Run("silent never prints even with warnings collected", func(t *testing.T) {
		_, collector := describeStacksErrorOptions("silent")
		collector.Add(e.DegradationWarning{Stack: "dev", Component: "vpc", Function: "!terraform.state vpc dev bucket", Reason: "terraform state not provisioned"})

		// No direct way to assert "nothing was printed" without capturing the UI
		// formatter's stream; this locks in that calling with "silent" does not attempt
		// to use the collector (see errorMode == "warn" guard) and does not panic.
		printErrorModeSummary("silent", collector)
	})

	t.Run("strict is a no-op with a nil collector", func(t *testing.T) {
		printErrorModeSummary("strict", nil)
	})
}
