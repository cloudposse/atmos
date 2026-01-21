package stack

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/schema"
)

var convertParser *flags.StandardParser

// ConvertOptions contains parsed flags for the convert command.
type ConvertOptions struct {
	global.Flags
	ToFormat   string
	OutputPath string
	DryRun     bool
}

// convertCmd converts stack configuration files between formats.
var convertCmd = &cobra.Command{
	Use:   "convert <input-file>",
	Short: "Convert stack configuration between formats",
	Long: `Convert stack configuration files between YAML, JSON, and HCL formats.

All formats parse to the same internal data structure, enabling seamless
bidirectional conversion between any supported format. Supports multi-document
YAML files and multi-stack HCL files.`,
	Example: markdown.StackConvertUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := convertParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &ConvertOptions{
			Flags:      flags.ParseGlobalFlags(cmd, v),
			ToFormat:   v.GetString("to"),
			OutputPath: v.GetString("output"),
			DryRun:     v.GetBool("dry-run"),
		}

		// Build ConfigAndStacksInfo with global flags.
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			BasePath:                opts.BasePath,
			AtmosConfigDirsFromArg:  opts.ConfigPath,
			AtmosConfigFilesFromArg: opts.Config,
			LogsLevel:               opts.LogsLevel,
			LogsFile:                opts.LogsFile,
		}

		// Load atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			// Config is optional for this command - we just need file conversion.
			atmosConfig = schema.AtmosConfiguration{}
		}

		return e.ExecuteStackConvert(&atmosConfig, args[0], opts.ToFormat, opts.OutputPath, opts.DryRun)
	},
}

func init() {
	// Create parser with convert-specific flags.
	convertParser = flags.NewStandardParser(
		flags.WithStringFlag("to", "t", "", "Target format: yaml, json, or hcl (required)"),
		flags.WithStringFlag("output", "o", "", "Output file path (optional, defaults to stdout)"),
		flags.WithBoolFlag("dry-run", "n", false, "Preview conversion without writing"),
		flags.WithEnvVars("to", "ATMOS_STACK_CONVERT_TO"),
		flags.WithEnvVars("output", "ATMOS_STACK_CONVERT_OUTPUT"),
		flags.WithEnvVars("dry-run", "ATMOS_STACK_CONVERT_DRY_RUN"),
	)

	// Register flags.
	convertParser.RegisterFlags(convertCmd)

	// Mark --to as required.
	_ = convertCmd.MarkFlagRequired("to")

	// Bind flags to Viper for environment variable support.
	if err := convertParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
