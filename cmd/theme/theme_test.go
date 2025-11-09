package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestThemeCommand(t *testing.T) {
	t.Run("theme command exists", func(t *testing.T) {
		assert.Equal(t, "theme", themeCmd.Use)
		assert.NotEmpty(t, themeCmd.Short)
		assert.NotEmpty(t, themeCmd.Long)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		hasListCmd := false
		for _, subCmd := range themeCmd.Commands() {
			if subCmd.Use == "list" {
				hasListCmd = true
				break
			}
		}
		assert.True(t, hasListCmd, "theme command should have list subcommand")
	})

	t.Run("has show subcommand", func(t *testing.T) {
		hasShowCmd := false
		for _, subCmd := range themeCmd.Commands() {
			if subCmd.Use == "show [theme-name]" {
				hasShowCmd = true
				break
			}
		}
		assert.True(t, hasShowCmd, "theme command should have show subcommand")
	})
}

func TestSetAtmosConfig(t *testing.T) {
	t.Run("sets config successfully", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "dracula",
				},
			},
		}

		SetAtmosConfig(config)
		assert.Equal(t, config, atmosConfigPtr)
	})

	t.Run("handles nil config", func(t *testing.T) {
		SetAtmosConfig(nil)
		assert.Nil(t, atmosConfigPtr)
	})
}

func TestThemeCommandProvider(t *testing.T) {
	provider := &ThemeCommandProvider{}

	t.Run("GetCommand returns theme command", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "theme", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		name := provider.GetName()
		assert.Equal(t, "theme", name)
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		group := provider.GetGroup()
		assert.Equal(t, "Other Commands", group)
	})
}

func TestThemeListCommand(t *testing.T) {
	t.Run("list command exists", func(t *testing.T) {
		assert.Equal(t, "list", themeListCmd.Use)
		assert.NotEmpty(t, themeListCmd.Short)
	})

	t.Run("has recommended flag", func(t *testing.T) {
		flag := themeListCmd.Flags().Lookup("recommended")
		require.NotNil(t, flag, "list command should have --recommended flag")
		assert.Equal(t, "bool", flag.Value.Type())
	})
}

func TestThemeShowCommand(t *testing.T) {
	t.Run("show command exists", func(t *testing.T) {
		assert.Equal(t, "show [theme-name]", themeShowCmd.Use)
		assert.NotEmpty(t, themeShowCmd.Short)
		assert.NotEmpty(t, themeShowCmd.Long)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		// Validate Args is set to ExactArgs(1)
		err := themeShowCmd.Args(themeShowCmd, []string{})
		assert.Error(t, err, "show command should require exactly one argument")

		err = themeShowCmd.Args(themeShowCmd, []string{"dracula"})
		assert.NoError(t, err, "show command should accept one argument")

		err = themeShowCmd.Args(themeShowCmd, []string{"dracula", "extra"})
		assert.Error(t, err, "show command should reject more than one argument")
	})
}
