package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// bindAuthParserRootLike registers parser's flags on a fresh command and binds them to a Viper that
// mimics the root command (SetEnvPrefix("ATMOS") + AutomaticEnv), which resolves ATMOS_<KEY> for
// any bare key before explicitly bound environment variables.
func bindAuthParserRootLike(t *testing.T, parser *flags.StandardParser, flagValue string) *viper.Viper {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)
	if flagValue != "" {
		require.NoError(t, cmd.Flags().Set(tagsFlagName, flagValue))
	}

	v := viper.New()
	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()
	require.NoError(t, parser.BindFlagsToViper(cmd, v))
	return v
}

// TestAuthTagsViperKey_IgnoresTerraformEnvVar proves a job-level ATMOS_TAGS (the terraform --tags
// variable) cannot select or filter auth identities, while ATMOS_AUTH_TAGS and --tags still work.
func TestAuthTagsViperKey_IgnoresTerraformEnvVar(t *testing.T) {
	parsers := map[string]*flags.StandardParser{
		"login":  loginParser,
		"list":   listParser,
		"logout": logoutParser,
	}

	for name, parser := range parsers {
		t.Run(name+"/ATMOS_TAGS does not leak", func(t *testing.T) {
			t.Setenv("ATMOS_TAGS", "admin")

			v := bindAuthParserRootLike(t, parser, "")

			assert.Empty(t, v.GetString(authTagsViperKey))
		})

		t.Run(name+"/ATMOS_AUTH_TAGS is honoured", func(t *testing.T) {
			t.Setenv("ATMOS_TAGS", "leak")
			t.Setenv("ATMOS_AUTH_TAGS", "production")

			v := bindAuthParserRootLike(t, parser, "")

			assert.Equal(t, "production", v.GetString(authTagsViperKey))
		})

		t.Run(name+"/--tags wins over the environment", func(t *testing.T) {
			t.Setenv("ATMOS_TAGS", "leak")
			t.Setenv("ATMOS_AUTH_TAGS", "env")

			v := bindAuthParserRootLike(t, parser, "cli")

			assert.Equal(t, "cli", v.GetString(authTagsViperKey))
		})
	}
}
