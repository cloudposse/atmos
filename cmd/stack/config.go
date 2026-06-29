package stack

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	pkgstack "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

var stackConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Read, edit, and list component config in stack manifests",
	Long: `Read, edit, and list component-relative config paths for a component in a
stack. Edits use provenance to target the manifest that defines the effective
value.`,
	Args: cobra.NoArgs,
}

var stackConfigGetCmd = &cobra.Command{
	Use:     "get <path>",
	Short:   "Read a component-relative value from a stack",
	Example: "atmos stack config get vars.region -s plat-ue2-prod -c vpc",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.config.getRunE")()
		return runStackGet(args)
	},
}

var stackConfigSetCmd = &cobra.Command{
	Use:     "set <path> <value>",
	Short:   "Set a component-relative value in the manifest that defines it",
	Example: "atmos stack config set vars.region us-west-2 -s plat-ue2-prod -c vpc",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.config.setRunE")()
		return runStackSet(args)
	},
}

var stackConfigDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Aliases: []string{"del", "unset"},
	Short:   "Delete a component-relative value from the manifest that defines it",
	Example: "atmos stack config delete settings.spacelift.workspace_enabled -s plat-ue2-prod -c vpc",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.config.deleteRunE")()
		return runStackDelete(args)
	},
}

var stackConfigFormatCmd = &cobra.Command{
	Use:     "format",
	Aliases: []string{"fmt"},
	Short:   "Format the manifest files that define a stack component",
	Example: "atmos stack config format -s plat-ue2-prod -c vpc\natmos stack config format -s plat-ue2-prod -c vpc --file stacks/catalog/vpc.yaml",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.config.formatRunE")()
		return runStackFormat()
	},
}

var stackConfigListCmd = &cobra.Command{
	Use:     "list [path-pattern]",
	Short:   "List editable component config paths for a stack",
	Example: "atmos stack config list -s plat-ue2-prod -c vpc\natmos stack config list 'vars.*' -s plat-ue2-prod -c vpc\natmos stack config list -s plat-ue2-prod -c vpc --format json",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.config.listRunE")()

		rows, err := runStackConfigList()
		if err != nil {
			return err
		}
		output, err := listpkg.RenderPathRowsWithPattern(rows, flagFormat, flagDelimiter, stackPathPatternArg(args))
		if err != nil {
			return err
		}
		return data.Write(output)
	},
}

func stackPathPatternArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func init() {
	for _, c := range []*cobra.Command{stackConfigGetCmd, stackConfigSetCmd, stackConfigDeleteCmd, stackConfigFormatCmd} {
		registerStackEditFlags(c)
	}
	stackConfigSetCmd.Flags().StringVar(&flagType, "type", atmosyaml.TypeString,
		"Value type: string, int, bool, float, null, or yaml (raw literal)")

	registerStackEditFlags(stackConfigListCmd)
	stackConfigListCmd.Flags().StringVarP(&flagFormat, "format", "f", "paths", "Output format: paths, table, json, yaml, csv, tsv")
	stackConfigListCmd.Flags().StringVar(&flagDelimiter, "delimiter", "", "Delimiter for csv/tsv output")

	stackConfigCmd.AddCommand(stackConfigGetCmd)
	stackConfigCmd.AddCommand(stackConfigSetCmd)
	stackConfigCmd.AddCommand(stackConfigDeleteCmd)
	stackConfigCmd.AddCommand(stackConfigFormatCmd)
	stackConfigCmd.AddCommand(stackConfigListCmd)
}

func runStackConfigList() ([]listpkg.PathRow, error) {
	if flagFile != "" {
		return buildStackConfigRowsFromFile(flagFile, "")
	}

	atmosConfig, result, err := describeComponentForEdit()
	if err != nil {
		return nil, err
	}
	return buildStackConfigRowsFromDescribe(&atmosConfig, result, flagComponent)
}

func buildStackConfigRowsFromDescribe(atmosConfig *schema.AtmosConfiguration, result *exec.DescribeComponentResult, component string) ([]listpkg.PathRow, error) {
	sectionYAML, err := u.ConvertToYAML(result.ComponentSection)
	if err != nil {
		return nil, err
	}
	entries, err := atmosyaml.ListPathEntries([]byte(sectionYAML))
	if err != nil {
		return nil, err
	}

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)
	rows := make([]listpkg.PathRow, 0, len(entries))
	for _, entry := range entries {
		file, ok := provenanceFileForComponentPath(atmosConfig, result, componentType, component, entry.Path)
		if !ok {
			continue
		}
		rows = append(rows, listpkg.PathRow{
			File:  file,
			Path:  entry.Path,
			Type:  entry.Type,
			Value: entry.Value,
		})
	}
	return rows, nil
}

func provenanceFileForComponentPath(atmosConfig *schema.AtmosConfiguration, result *exec.DescribeComponentResult, componentType, component, dotPath string) (string, bool) {
	if result.MergeContext == nil {
		return "", false
	}
	entries := result.MergeContext.GetProvenance(pkgstack.BuildComponentInFilePath(componentType, component, dotPath))
	provFile, _, ok := pkgstack.PickProvenanceFile(entries)
	if !ok {
		return "", false
	}
	return relativePathForStackDisplay(provFile, atmosConfig.StacksBaseAbsolutePath), true
}

func buildStackConfigRowsFromFile(file string, basePath string) ([]listpkg.PathRow, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	entries, err := atmosyaml.ListPathEntries(content)
	if err != nil {
		return nil, err
	}

	displayFile := relativePathForStackDisplay(file, basePath)
	rows := make([]listpkg.PathRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, listpkg.PathRow{
			File:  displayFile,
			Path:  entry.Path,
			Type:  entry.Type,
			Value: entry.Value,
		})
	}
	return rows, nil
}

func relativePathForStackDisplay(file, basePath string) string {
	if basePath == "" || !filepath.IsAbs(file) {
		return filepath.ToSlash(file)
	}
	rel, err := filepath.Rel(basePath, file)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}
