package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// TestDescribeCmd_IdentityFlagNoOptDefVal verifies describeCmd registers --identity via the
// shared flags.WithIdentityFlag() builder, so a bare --identity (no value) carries the
// interactive-selection sentinel instead of failing with "flag needs an argument".
func TestDescribeCmd_IdentityFlagNoOptDefVal(t *testing.T) {
	identityFlag := describeCmd.PersistentFlags().Lookup("identity")
	require.NotNil(t, identityFlag, "identity flag should be registered on the describe command")
	assert.Equal(t, cfg.IdentityFlagSelectValue, identityFlag.NoOptDefVal,
		"bare --identity must carry the select sentinel, matching every other Atmos command")
	assert.Equal(t, "i", identityFlag.Shorthand)
}
