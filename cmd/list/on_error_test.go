package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
)

func TestDescribeStacksErrorOptions(t *testing.T) {
	t.Run("strict by default", func(t *testing.T) {
		opts := describeStacksErrorOptions("strict")
		assert.Equal(t, e.DescribeStacksErrorOptions{}, opts)
	})

	t.Run("empty value is strict", func(t *testing.T) {
		opts := describeStacksErrorOptions("")
		assert.Equal(t, e.DescribeStacksErrorOptions{}, opts)
	})

	t.Run("warn enables OnErrorWarn and a non-nil callback", func(t *testing.T) {
		opts := describeStacksErrorOptions("warn")
		require.Equal(t, e.OnErrorWarn, opts.OnError)
		require.NotNil(t, opts.OnWarning)

		// The callback must not panic when invoked with a warning.
		opts.OnWarning(e.DegradationWarning{
			Stack:     "dev",
			Component: "vpc",
			Function:  "!terraform.state vpc dev bucket",
			Reason:    "terraform state not provisioned",
		})
	})
}
