package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

var configListParser *flags.StandardParser

var configListCmd = &cobra.Command{
	Use:   "list [path-pattern]",
	Short: "List editable atmos.yaml setting paths",
	Long: `List the physical Atmos config files that contributed settings and the
dot-notation setting paths defined in each file. Optionally filter paths with a glob pattern.`,
	Example: "atmos config list\natmos config list 'toolchain.*'\natmos config list --format json",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.listRunE")()

		v := viper.GetViper()
		if err := configListParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		rows, err := buildConfigPathRows(cfg.LoadedConfigFiles(), atmosConfig.BasePathAbsolute)
		if err != nil {
			return err
		}

		output, err := listpkg.RenderPathRowsWithPattern(rows, v.GetString("format"), v.GetString("delimiter"), pathPatternArg(args))
		if err != nil {
			return err
		}
		return data.Write(output)
	},
}

func pathPatternArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func init() {
	configListParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "paths", "Output format: paths, table, json, yaml, csv, tsv"),
		flags.WithStringFlag("delimiter", "", "", "Delimiter for csv/tsv output"),
	)
	configListParser.RegisterFlags(configListCmd)
	if err := configListParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func buildConfigPathRows(files []string, basePath string) ([]listpkg.PathRow, error) {
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

		displayFile := relativePathForDisplay(file, basePath)
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

func relativePathForDisplay(file, basePath string) string {
	if basePath == "" {
		return filepath.ToSlash(file)
	}
	rel, err := filepath.Rel(basePath, file)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}
