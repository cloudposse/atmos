package helm

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const directRepositoryName = "direct"

type repoListOptions struct {
	component        string
	stack            string
	format           string
	columns          []string
	processTemplates bool
	processFunctions bool
	skip             []string
}

func newRepoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage Helm repositories associated with native Helm components",
	}
	cmd.AddCommand(newRepoListCommand())
	return cmd
}

func newRepoListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [component]",
		Short: "List Helm repositories associated with native Helm components",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := repoListOptions{
				processTemplates: true,
				processFunctions: true,
			}
			if len(args) > 0 {
				opts.component = args[0]
			}
			opts.stack = commandFlagString(cmd, "stack")
			opts.format, _ = cmd.Flags().GetString("format")
			opts.columns, _ = cmd.Flags().GetStringSlice("columns")
			opts.processTemplates, _ = cmd.Flags().GetBool("process-templates")
			opts.processFunctions, _ = cmd.Flags().GetBool("process-functions")
			opts.skip, _ = cmd.Flags().GetStringSlice("skip")
			return runRepoList(cmd, &opts)
		},
	}
	cmd.Flags().StringP("format", "f", "", "Output format: table, json, yaml, csv, tsv")
	cmd.Flags().StringSlice("columns", []string{}, "Columns to display (comma-separated)")
	cmd.Flags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests")
	cmd.Flags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests")
	cmd.Flags().StringSlice("skip", []string{}, "Skip paths when loading stack manifests")
	return cmd
}

func commandFlagString(cmd *cobra.Command, name string) string {
	if flag := cmd.Flag(name); flag != nil {
		return flag.Value.String()
	}
	return ""
}

func runRepoList(cmd *cobra.Command, opts *repoListOptions) error {
	atmosConfig, stacksMap, err := repoListConfigAndStacks(cmd, opts)
	if err != nil {
		return err
	}

	rows := helmRepositoryRows(atmosConfig, stacksMap, opts.component)
	if len(rows) == 0 {
		ui.Info("No Helm repositories found")
		return nil
	}

	return renderRepoListRows(rows, opts)
}

func repoListConfigAndStacks(cmd *cobra.Command, opts *repoListOptions) (*schema.AtmosConfiguration, map[string]any, error) {
	info := buildConfigAndStacksInfo(cmd)
	info.Command = cfg.HelmComponentType
	info.SubCommand = "repo list"
	info.Stack = opts.stack
	info.ProcessTemplates = opts.processTemplates
	info.ProcessFunctions = opts.processFunctions
	info.Skip = opts.skip

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return nil, nil, err
	}

	authManager, err := auth.CreateAndAuthenticateManagerWithStackScan(
		info.Identity,
		&atmosConfig.Auth,
		cfg.IdentityFlagSelectValue,
		&atmosConfig,
	)
	if err != nil {
		return nil, nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(
		&atmosConfig,
		opts.stack,
		nil,
		[]string{cfg.HelmComponentType},
		nil,
		false,
		opts.processTemplates,
		opts.processFunctions,
		false,
		opts.skip,
		authManager,
	)
	if err != nil {
		return nil, nil, err
	}

	return &atmosConfig, stacksMap, nil
}

func renderRepoListRows(rows []map[string]any, opts *repoListOptions) error {
	selector, err := column.NewSelector(defaultRepoListColumns(), column.BuildColumnFuncMap())
	if err != nil {
		return err
	}
	if err := selector.Select(opts.columns); err != nil {
		return err
	}

	outputFormat, err := repoListFormat(opts.format)
	if err != nil {
		return err
	}

	return renderer.New(nil, selector, nil, outputFormat, "").Render(rows)
}

func repoListFormat(value string) (format.Format, error) {
	outputFormat := format.Format(value)
	switch outputFormat {
	case "", format.FormatTable, format.FormatJSON, format.FormatYAML, format.FormatCSV, format.FormatTSV:
		return outputFormat, nil
	default:
		return "", fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidConfig, outputFormat)
	}
}

func defaultRepoListColumns() []column.Config {
	return []column.Config{
		{Name: "stack", Value: "{{ .stack }}"},
		{Name: "component", Value: "{{ .component }}"},
		{Name: "name", Value: "{{ .name }}"},
		{Name: "url", Value: "{{ .url }}"},
		{Name: "source", Value: "{{ .source }}"},
		{Name: "chart", Value: "{{ .chart }}"},
		{Name: "used", Value: "{{ .used }}"},
	}
}

func helmRepositoryRows(atmosConfig *schema.AtmosConfiguration, stacksMap map[string]any, componentFilter string) []map[string]any {
	stackNames := make([]string, 0, len(stacksMap))
	for stackName := range stacksMap {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	rows := make([]map[string]any, 0)
	for _, stackName := range stackNames {
		helmComponents, ok := helmComponentsForStack(stacksMap[stackName])
		if !ok {
			continue
		}

		for _, componentName := range repoListComponentNames(helmComponents, componentFilter) {
			section, ok := helmComponents[componentName].(map[string]any)
			if !ok {
				continue
			}
			rows = append(rows, rowsForComponentRepositories(atmosConfig, stackName, componentName, section)...)
		}
	}
	return rows
}

func helmComponentsForStack(stackData any) (map[string]any, bool) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, false
	}
	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, false
	}
	helmComponents, ok := componentsMap[cfg.HelmComponentType].(map[string]any)
	return helmComponents, ok
}

func repoListComponentNames(helmComponents map[string]any, componentFilter string) []string {
	componentNames := make([]string, 0, len(helmComponents))
	for componentName := range helmComponents {
		if componentFilter == "" || componentName == componentFilter {
			componentNames = append(componentNames, componentName)
		}
	}
	sort.Strings(componentNames)
	return componentNames
}

func rowsForComponentRepositories(
	atmosConfig *schema.AtmosConfiguration,
	stackName string,
	componentName string,
	section map[string]any,
) []map[string]any {
	chart := repoListStringField(section, cfg.ChartSectionName)
	rows := make([]map[string]any, 0)

	if directURL := repoListStringField(section, "repository"); directURL != "" {
		rows = append(rows, map[string]any{
			"stack":     stackName,
			"component": componentName,
			"name":      directRepositoryName,
			"url":       directURL,
			"source":    "direct",
			"chart":     chart,
			"used":      true,
		})
	}

	repositories := mergeRepoListRepositories(atmosConfig, section)
	for i := range repositories {
		repo := &repositories[i]
		rows = append(rows, map[string]any{
			"stack":     stackName,
			"component": componentName,
			"name":      repo.Name,
			"url":       repo.URL,
			"source":    repo.Source,
			"chart":     chart,
			"used":      chartUsesRepository(chart, repo.Name),
		})
	}
	return rows
}

type repoListRepository struct {
	Name   string
	URL    string
	Source string
}

func mergeRepoListRepositories(atmosConfig *schema.AtmosConfiguration, section map[string]any) []repoListRepository {
	out := make([]repoListRepository, 0)
	positions := make(map[string]int)
	if atmosConfig != nil {
		for i := range atmosConfig.Components.Helm.Repositories {
			repo := &atmosConfig.Components.Helm.Repositories[i]
			if repo.Name == "" || repo.URL == "" {
				continue
			}
			positions[repo.Name] = len(out)
			out = append(out, repoListRepository{Name: repo.Name, URL: repo.URL, Source: "global"})
		}
	}
	for _, repo := range repoListRepositoriesFromSection(section) {
		if pos, ok := positions[repo.Name]; ok {
			out[pos] = repo
			continue
		}
		positions[repo.Name] = len(out)
		out = append(out, repo)
	}
	return out
}

func repoListRepositoriesFromSection(section map[string]any) []repoListRepository {
	out := make([]repoListRepository, 0)
	for _, entry := range repoListAnySlice(section[cfg.RepositoriesSectionName]) {
		repo, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		item := repoListRepository{
			Name:   repoListStringField(repo, "name"),
			URL:    repoListStringField(repo, "url"),
			Source: "component",
		}
		if item.Name != "" && item.URL != "" {
			out = append(out, item)
		}
	}
	return out
}

func chartUsesRepository(chart, repoName string) bool {
	if chart == "" || repoName == "" {
		return false
	}
	if strings.HasPrefix(chart, ".") || filepath.IsAbs(chart) || strings.HasPrefix(chart, "oci://") {
		return false
	}
	name, _, ok := strings.Cut(chart, "/")
	return ok && name == repoName
}

func repoListStringField(section map[string]any, key string) string {
	value, _ := section[key].(string)
	return value
}

func repoListAnySlice(value any) []any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		return typed
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}
