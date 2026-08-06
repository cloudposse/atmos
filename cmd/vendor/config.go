package vendor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// Each vendor config subcommand owns an independent StandardParser: earlier revisions shared
// package-level vars (vendorConfigFileFlag, vendorConfigType, vendorConfigFormat,
// vendorConfigDelimiter) across commands, which meant setting one subcommand's flag could leak
// into another's default.
var (
	vendorConfigGetParser    *flags.StandardParser
	vendorConfigSetParser    *flags.StandardParser
	vendorConfigDeleteParser *flags.StandardParser
	vendorConfigFormatParser *flags.StandardParser
	vendorConfigListParser   *flags.StandardParser
)

var vendorConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Read, edit, and list raw vendor manifest config",
	Long: `Read, edit, and list raw vendor manifest settings using dot-notation paths.
Use the existing vendor get/set commands for the component-version shortcut.`,
	Args: cobra.NoArgs,
}

var vendorConfigGetCmd = &cobra.Command{
	Use:     "get <path>",
	Short:   "Read a raw value from vendor.yaml by dot-notation path",
	Example: "atmos vendor config get spec.sources[0].version",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.getRunE")()

		file, err := resolveVendorFileFromCmd(cmd)
		if err != nil {
			return err
		}
		return runVendorConfigGet(file, args[0])
	},
}

// runVendorConfigGet reads the raw value at path from a vendor manifest file
// and writes it to stdout. Shared by vendor config get and its vendor get
// alias.
func runVendorConfigGet(file, path string) error {
	value, err := atmosyaml.GetFile(file, path)
	if err != nil {
		return err
	}
	return data.Writeln(value)
}

var vendorConfigSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a raw value in vendor.yaml by dot-notation path",
	Long: `Set a raw value in vendor.yaml using dot-notation paths. Values default to
strings; use --type for int, bool, float, null, or raw YAML literals.`,
	Example: "atmos vendor config set spec.sources[0].version v1.2.3",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.setRunE")()

		file, err := resolveVendorFileFromCmd(cmd)
		if err != nil {
			return err
		}
		valueType, err := cmd.Flags().GetString("type")
		if err != nil {
			return err
		}
		return runVendorConfigSet(file, args[0], args[1], valueType)
	},
}

// runVendorConfigSet writes value at path in a vendor manifest file. Shared by
// vendor config set and its vendor set alias.
func runVendorConfigSet(file, path, value, valueType string) error {
	created, err := atmosyaml.SetFileWithType(file, path, value, valueType)
	if err != nil {
		return err
	}
	if created {
		ui.Successf("Created `%s` = `%s` in `%s`", path, value, atmosyaml.DisplayPath(file))
		return nil
	}
	ui.Successf("Updated `%s` to `%s` in `%s`", path, value, atmosyaml.DisplayPath(file))
	return nil
}

var vendorConfigDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Aliases: []string{"del", "unset"},
	Short:   "Delete a raw value from vendor.yaml by dot-notation path",
	Example: "atmos vendor config delete spec.sources[0].tags",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.deleteRunE")()

		file, err := resolveVendorFileFromCmd(cmd)
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

var vendorConfigFormatCmd = &cobra.Command{
	Use:     "format",
	Aliases: []string{"fmt"},
	Short:   "Format vendor manifest config files",
	Long: `Format the root vendor manifest and imported vendor manifest files in place,
preserving comments, anchors, YAML functions, and templates.`,
	Example: "atmos vendor config format\natmos vendor config format --file ./vendor.yaml",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.formatRunE")()

		file, err := resolveVendorFileFromCmd(cmd)
		if err != nil {
			return err
		}
		files, err := formatVendorConfigFiles(file)
		if err != nil {
			return err
		}
		ui.Successf("Formatted %d vendor config file(s).", len(files))
		return nil
	},
}

var vendorConfigListCmd = &cobra.Command{
	Use:     "list [path-pattern]",
	Short:   "List raw vendor manifest setting paths",
	Example: "atmos vendor config list\natmos vendor config list 'spec.sources[*].version'\natmos vendor config list --format json",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.listRunE")()

		file, err := resolveVendorFileFromCmd(cmd)
		if err != nil {
			return err
		}
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		delimiter, err := cmd.Flags().GetString("delimiter")
		if err != nil {
			return err
		}
		rows, err := buildVendorConfigPathRows(file)
		if err != nil {
			return err
		}
		output, err := listpkg.RenderPathRowsWithPattern(rows, format, delimiter, vendorPathPatternArg(args))
		if err != nil {
			return err
		}
		return data.Write(output)
	},
}

func vendorPathPatternArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func init() {
	vendorConfigGetParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", vendorFileFlagHelp),
	)
	vendorConfigGetParser.RegisterFlags(vendorConfigGetCmd)
	if err := vendorConfigGetParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorConfigSetParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", vendorFileFlagHelp),
		flags.WithStringFlag("type", "", atmosyaml.TypeString, "Value type: string, int, bool, float, null, or yaml (raw literal)"),
	)
	vendorConfigSetParser.RegisterFlags(vendorConfigSetCmd)
	if err := vendorConfigSetParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorConfigDeleteParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", vendorFileFlagHelp),
	)
	vendorConfigDeleteParser.RegisterFlags(vendorConfigDeleteCmd)
	if err := vendorConfigDeleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorConfigFormatParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", vendorFileFlagHelp),
	)
	vendorConfigFormatParser.RegisterFlags(vendorConfigFormatCmd)
	if err := vendorConfigFormatParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorConfigListParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", vendorFileFlagHelp),
		flags.WithStringFlag("format", "f", "paths", "Output format: paths, table, json, yaml, csv, tsv"),
		flags.WithStringFlag("delimiter", "", "", "Delimiter for csv/tsv output"),
	)
	vendorConfigListParser.RegisterFlags(vendorConfigListCmd)
	if err := vendorConfigListParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorConfigCmd.AddCommand(vendorConfigGetCmd)
	vendorConfigCmd.AddCommand(vendorConfigSetCmd)
	vendorConfigCmd.AddCommand(vendorConfigDeleteCmd)
	vendorConfigCmd.AddCommand(vendorConfigFormatCmd)
	vendorConfigCmd.AddCommand(vendorConfigListCmd)
	vendorCmd.AddCommand(vendorConfigCmd)
}

func buildVendorConfigPathRows(rootFile string) ([]listpkg.PathRow, error) {
	files, err := vendoring.CollectManifestFiles(rootFile)
	if err != nil {
		return nil, err
	}

	basePath := filepath.Dir(rootFile)
	rows := make([]listpkg.PathRow, 0)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		entries, err := atmosyaml.ListPathEntries(content)
		if err != nil {
			return nil, err
		}
		displayFile := relativeVendorPathForDisplay(file, basePath)
		for _, entry := range entries {
			rows = append(rows, listpkg.PathRow{
				File:  displayFile,
				Path:  entry.Path,
				Type:  entry.Type,
				Value: entry.Value,
			})
		}
	}
	return rows, nil
}

func formatVendorConfigFiles(rootFile string) ([]string, error) {
	files, err := vendoring.CollectManifestFiles(rootFile)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if err := atmosyaml.FormatFile(file); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func relativeVendorPathForDisplay(file, basePath string) string {
	rel, err := filepath.Rel(basePath, file)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}
