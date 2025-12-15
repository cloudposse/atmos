package theme

import (
	_ "embed"
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ThemesArgCompletion provides auto-completion for theme names.
func ThemesArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "theme.ThemesArgCompletion")()

	registry, err := theme.NewRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	themes := registry.List()
	names := make([]string, 0, len(themes))
	for _, t := range themes {
		names = append(names, t.Name)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

//go:embed markdown/atmos_theme_show_usage.md
var themeShowUsage string

// themeShowParser is the flag parser for theme show command.
var themeShowParser *flags.StandardFlagParser

// ThemeShowOptions holds the options for theme show command.
type ThemeShowOptions struct {
	ThemeName string
}

// themeShowCmd shows details and preview of a specific theme.
var themeShowCmd = &cobra.Command{
	Use:     "show",
	Short:   "Show details and preview of a specific theme",
	Long:    "Display color palette and sample UI elements for a specific terminal theme.",
	Example: themeShowUsage,
	// Args validator will be set by positional args builder.
	RunE: executeThemeShow,
}

func init() {
	// Build positional args specification.
	builder := flags.NewPositionalArgsBuilder()
	builder.AddArg(&flags.PositionalArgSpec{
		Name:        "theme-name",
		Description: "Theme name to preview",
		Required:    true,
		TargetField: "ThemeName",
	})
	specs, validator, usage := builder.Build()

	// Create flag parser with interactive prompt config.
	themeShowParser = flags.NewStandardFlagParser(
		flags.WithPositionalArgPrompt("theme-name", "Choose a theme to preview", ThemesArgCompletion),
	)

	// Set positional args configuration.
	themeShowParser.SetPositionalArgs(specs, validator, usage)

	// Update command's Use field with positional args usage.
	themeShowCmd.Use = "show " + usage

	// Register flags with cobra (registers positional arg completion).
	// The flag handler will set a prompt-aware Args validator automatically
	// when prompts are configured, allowing missing args to trigger prompts.
	themeShowParser.RegisterFlags(themeShowCmd)

	// Bind both env vars and pflags to viper for full precedence support (flag > env > config > default).
	if err := themeShowParser.BindFlagsToViper(themeShowCmd, viper.GetViper()); err != nil {
		// Log error but don't fail initialization.
		// This allows the command to still work even if Viper binding fails.
		_ = err
	}

	themeCmd.AddCommand(themeShowCmd)
}

// executeThemeShow displays detailed information about a specific theme.
//
// Note: This command is a preview/demo that shows what theme styles look like.
// The ui.Write() call correctly outputs the preview to the UI channel (stderr).
func executeThemeShow(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "theme.show.RunE")()

	// Parse flags and positional args (handles interactive prompts).
	parsed, err := themeShowParser.Parse(cmd.Context(), args)
	if err != nil {
		return err
	}

	// Extract theme name from positional args.
	if len(parsed.PositionalArgs) == 0 {
		return errUtils.Build(errUtils.ErrInvalidPositionalArgs).
			WithExplanation("Theme name is required").
			WithHintf("Run `atmos list themes` to see all available themes").
			WithHint("Browse themes at https://atmos.tools/cli/commands/theme/browse").
			WithExitCode(2).
			Err()
	}

	opts := &ThemeShowOptions{
		ThemeName: parsed.PositionalArgs[0],
	}

	result := theme.ShowTheme(theme.ShowThemeOptions{
		ThemeName: opts.ThemeName,
	})

	if result.Error != nil {
		// Check if it's a theme not found error and enrich it.
		if errors.Is(result.Error, theme.ErrThemeNotFound) {
			return errUtils.Build(errUtils.ErrThemeNotFound).
				WithHintf("Run `atmos list themes` to see all available themes").
				WithHint("Browse themes at https://atmos.tools/cli/commands/theme/browse").
				WithExitCode(2).
				Err()
		}
		// Pass through other errors.
		return result.Error
	}

	return ui.Write(result.Output)
}
