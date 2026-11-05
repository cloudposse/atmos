package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescribeConfigCmd_Error(t *testing.T) {
	NewTestKit(t)
	// Cobra rejects unknown flags before RunE; calling RunE directly bypasses
	// parsing and can fail for an unrelated, uninitialized inherited flag.
	err := describeConfigCmd.ParseFlags([]string{"--invalid-flag"})
	require.ErrorContains(t, err, "unknown flag: --invalid-flag")
}
