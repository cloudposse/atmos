package exec

import (
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/require"
)

func TestGlobalMetadataErrorHasOneActionableHint(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, err := validateGlobalMetadataSection(map[string]any{"component": "vpc"}, "dev")
	require.Error(t, err)
	output := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	require.Equal(t, 1, strings.Count(output, "💡"))
	require.Contains(t, output, "Global metadata only supports")
	require.Contains(t, output, "Move component-identity fields")
	require.NotContains(t, output, "# Error")
	require.NotContains(t, output, "**Error:**")
}
