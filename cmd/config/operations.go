package config

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// valueType holds the --type flag for `config set`.
var valueType string

var configGetCmd = &cobra.Command{
	Use:     "get <path>",
	Short:   "Read a value from atmos.yaml by dot-notation path",
	Long:    "Read a value from atmos.yaml using a dot-notation path (e.g. logs.level).",
	Example: "atmos config get logs.level",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.getRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		value, err := atmosyaml.GetFile(file, args[0])
		if err != nil {
			return err
		}
		return data.Writeln(value)
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a value in atmos.yaml by dot-notation path",
	Long: `Set a value in atmos.yaml using a dot-notation path, preserving comments,
anchors, YAML functions, and templates. The value's type (string, int, bool,
float) is inferred from the Atmos config schema when the path matches a known
field (e.g. mcp.enabled infers bool); pass --type explicitly to override, or
for paths the schema doesn't model (falls back to string).`,
	Example: "atmos config set logs.level debug\natmos config set mcp.enabled true\natmos config set --type=yaml logs.exclude '[\"a\", \"b\"]'",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.setRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		created, err := atmosyaml.SetFileWithType(file, args[0], args[1], effectiveValueType(cmd, args[0]))
		if err != nil {
			return err
		}
		if created {
			ui.Successf("Created `%s` = `%s` in `%s`", args[0], args[1], atmosyaml.DisplayPath(file))
			return nil
		}
		ui.Successf("Updated `%s` to `%s` in `%s`", args[0], args[1], atmosyaml.DisplayPath(file))
		return nil
	},
}

// effectiveValueType returns the --type flag's value when the user passed it
// explicitly. Otherwise it infers a type from the Atmos config schema for
// dotPath (e.g. a known bool field), falling back to the flag's default
// (atmosyaml.TypeString) when the path isn't modeled by the schema -- most
// commonly free-form sections like vars.
func effectiveValueType(cmd *cobra.Command, dotPath string) string {
	if cmd.Flags().Changed("type") {
		return valueType
	}
	if inferred, ok := cfg.InferValueType(dotPath); ok {
		return inferred
	}
	return valueType
}

var configDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Aliases: []string{"del", "unset"},
	Short:   "Delete a value from atmos.yaml by dot-notation path",
	Long:    "Delete a value from atmos.yaml using a dot-notation path, preserving the rest of the file.",
	Example: "atmos config delete components.terraform.append_user_agent",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.deleteRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		existed, err := atmosyaml.DeleteFile(file, args[0])
		if err != nil {
			return err
		}
		if !existed {
			ui.Successf("Nothing to delete — `%s` is not set in `%s`", args[0], atmosyaml.DisplayPath(file))
			return nil
		}
		ui.Successf("Deleted `%s` from `%s`", args[0], atmosyaml.DisplayPath(file))
		return nil
	},
}

var configFormatCmd = &cobra.Command{
	Use:     "format",
	Aliases: []string{"fmt"},
	Short:   "Format the active atmos.yaml file",
	Long: `Format the active atmos.yaml file in place, preserving comments, anchors,
Atmos YAML functions, and Go templates.`,
	Example: "atmos config format\natmos --config ./config/atmos.yaml config format",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.formatRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}
		if err := atmosyaml.FormatFile(file); err != nil {
			return err
		}
		ui.Successf("Formatted `%s`", atmosyaml.DisplayPath(file))
		return nil
	},
}

func init() {
	configSetCmd.Flags().StringVar(&valueType, "type", atmosyaml.TypeString,
		"Value type: string, int, bool, float, null, or yaml (raw literal). "+
			"Auto-inferred from the Atmos config schema when omitted and the path is recognized.")
}

// resolveConfigFile picks the atmos.yaml to edit. The inherited persistent
// --config flag (first entry) acts as an explicit override; otherwise the file
// is discovered in the current directory or git root.
func resolveConfigFile(cmd *cobra.Command) (string, error) {
	override := ""
	if cfgFiles, _ := cmd.Flags().GetStringSlice("config"); len(cfgFiles) > 0 {
		override = cfgFiles[0]
	}

	file, err := cfg.ResolveEditableConfigFile(atmosConfigPtr, override)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation(err.Error()).
			WithHint("Run from a directory containing atmos.yaml, or pass --config <file>.").
			Err()
	}
	return file, nil
}
