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
anchors, YAML functions, and templates. By default the value is written as a
string; use --type to write an int, bool, float, null, or raw YAML literal.`,
	Example: "atmos config set logs.level debug\natmos config set --type=bool settings.list_merge_strategy_disabled true",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.setRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		if err := atmosyaml.SetFileWithType(file, args[0], args[1], valueType); err != nil {
			return err
		}
		ui.Successf("Updated %s in %s", args[0], file)
		return nil
	},
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

		if err := atmosyaml.DeleteFile(file, args[0]); err != nil {
			return err
		}
		ui.Successf("Deleted %s from %s", args[0], file)
		return nil
	},
}

func init() {
	configSetCmd.Flags().StringVar(&valueType, "type", atmosyaml.TypeString,
		"Value type: string, int, bool, float, null, or yaml (raw literal)")
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
