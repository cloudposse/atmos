package flags

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// TestFlagValueFromEnv verifies that FlagValueFromEnv reports the environment variable that supplied a flag value.
func TestFlagValueFromEnv(t *testing.T) {
	t.Run("nil flag is not env sourced", func(t *testing.T) {
		envVar, ok := FlagValueFromEnv(nil)
		assert.False(t, ok)
		assert.Empty(t, envVar)
	})

	t.Run("flag without annotations is not env sourced", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("tags", "", "")
		envVar, ok := FlagValueFromEnv(fs.Lookup("tags"))
		assert.False(t, ok)
		assert.Empty(t, envVar)
	})

	t.Run("marked flag reports its env var", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("tags", "", "")
		f := fs.Lookup("tags")
		MarkFlagValueFromEnv(f, "ATMOS_TAGS")
		envVar, ok := FlagValueFromEnv(f)
		assert.True(t, ok)
		assert.Equal(t, "ATMOS_TAGS", envVar)
	})

	t.Run("marking only affects the marked flag", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("tags", "", "")
		fs.String("labels", "", "")
		MarkFlagValueFromEnv(fs.Lookup("tags"), "ATMOS_TAGS")
		_, ok := FlagValueFromEnv(fs.Lookup("labels"))
		assert.False(t, ok)
	})

	t.Run("nil flag is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { MarkFlagValueFromEnv(nil, "ATMOS_TAGS") })
	})

	t.Run("empty env var on an unmarked flag is a no-op", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("tags", "", "")
		f := fs.Lookup("tags")
		MarkFlagValueFromEnv(f, "")
		_, ok := FlagValueFromEnv(f)
		assert.False(t, ok)
	})

	t.Run("empty env var clears a previous mark", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.String("tags", "", "")
		f := fs.Lookup("tags")
		MarkFlagValueFromEnv(f, "ATMOS_TAGS")
		MarkFlagValueFromEnv(f, "")
		_, ok := FlagValueFromEnv(f)
		assert.False(t, ok)
	})
}
