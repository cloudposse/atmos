package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringArrayFlagPreservesRepeatedCommaValues(t *testing.T) {
	parser := NewStandardParser(WithStringArrayFlag("set", "s", nil, "Set values"))
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	require.NoError(t, cmd.Flags().Set("set", "labels=one,two"))
	require.NoError(t, cmd.Flags().Set("set", "image.tag=1.2.3"))

	flag := cmd.Flags().Lookup("set")
	require.NotNil(t, flag)
	assert.Equal(t, "stringArray", flag.Value.Type())
	values, err := cmd.Flags().GetStringArray("set")
	require.NoError(t, err)
	assert.Equal(t, []string{"labels=one,two", "image.tag=1.2.3"}, values)
}
