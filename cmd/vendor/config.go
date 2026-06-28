package vendor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

var (
	vendorConfigFileFlag  string
	vendorConfigType      string
	vendorConfigFormat    string
	vendorConfigDelimiter string
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

		file, err := resolveVendorConfigFile()
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

var vendorConfigSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a raw value in vendor.yaml by dot-notation path",
	Long: `Set a raw value in vendor.yaml using dot-notation paths. Values default to
strings; use --type for int, bool, float, null, or raw YAML literals.`,
	Example: "atmos vendor config set spec.sources[0].version v1.2.3",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.setRunE")()

		file, err := resolveVendorConfigFile()
		if err != nil {
			return err
		}
		if err := atmosyaml.SetFileWithType(file, args[0], args[1], vendorConfigType); err != nil {
			return err
		}
		ui.Successf("Updated %s in %s", args[0], file)
		return nil
	},
}

var vendorConfigDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Aliases: []string{"del", "unset"},
	Short:   "Delete a raw value from vendor.yaml by dot-notation path",
	Example: "atmos vendor config delete spec.sources[0].tags",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.deleteRunE")()

		file, err := resolveVendorConfigFile()
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

var vendorConfigListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List raw vendor manifest setting paths",
	Example: "atmos vendor config list\natmos vendor config list --format json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.config.listRunE")()

		file, err := resolveVendorConfigFile()
		if err != nil {
			return err
		}
		rows, err := buildVendorConfigPathRows(file)
		if err != nil {
			return err
		}
		output, err := listpkg.RenderPathRows(rows, vendorConfigFormat, vendorConfigDelimiter)
		if err != nil {
			return err
		}
		return data.Write(output)
	},
}

func init() {
	for _, c := range []*cobra.Command{vendorConfigGetCmd, vendorConfigSetCmd, vendorConfigDeleteCmd, vendorConfigListCmd} {
		c.Flags().StringVar(&vendorConfigFileFlag, "file", "", "Vendor manifest file (default: ./vendor.yaml)")
	}
	vendorConfigSetCmd.Flags().StringVar(&vendorConfigType, "type", atmosyaml.TypeString,
		"Value type: string, int, bool, float, null, or yaml (raw literal)")
	vendorConfigListCmd.Flags().StringVarP(&vendorConfigFormat, "format", "f", "paths", "Output format: paths, table, json, yaml, csv, tsv")
	vendorConfigListCmd.Flags().StringVar(&vendorConfigDelimiter, "delimiter", "", "Delimiter for csv/tsv output")

	vendorConfigCmd.AddCommand(vendorConfigGetCmd)
	vendorConfigCmd.AddCommand(vendorConfigSetCmd)
	vendorConfigCmd.AddCommand(vendorConfigDeleteCmd)
	vendorConfigCmd.AddCommand(vendorConfigListCmd)
	vendorCmd.AddCommand(vendorConfigCmd)
}

func resolveVendorConfigFile() (string, error) {
	return resolveVendorFileWithOverride(vendorConfigFileFlag)
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

func relativeVendorPathForDisplay(file, basePath string) string {
	rel, err := filepath.Rel(basePath, file)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}
