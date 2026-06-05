package migrate

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/list/column"
	listFilter "github.com/cloudposse/atmos/pkg/list/filter"
	listFormat "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

const (
	migrateListFlagFormat    = "format"
	migrateListFlagColumns   = "columns"
	migrateListFlagSort      = "sort"
	migrateListFlagDelimiter = "delimiter"

	tfmigrateListKeyComponent        = "component"
	tfmigrateListKeyConfig           = "config"
	tfmigrateListKeyHistoryBucket    = "history_bucket"
	tfmigrateListKeyHistoryEndpoint  = "history_endpoint"
	tfmigrateListKeyHistoryKMSKeyID  = "history_kms_key_id"
	tfmigrateListKeyHistoryKey       = "history_key"
	tfmigrateListKeyHistoryNamespace = "history_namespace"
	tfmigrateListKeyHistoryProfile   = "history_profile"
	tfmigrateListKeyHistoryRegion    = "history_region"
	tfmigrateListKeyHistoryRoleARN   = "history_role_arn"
	tfmigrateListKeyHistoryStorage   = "history_storage"
	tfmigrateListKeyHook             = "hook"
	tfmigrateListKeyKind             = "kind"
	tfmigrateListKeyMigration        = "migration"
	tfmigrateListKeyMode             = "mode"
	tfmigrateListKeyName             = "name"
	tfmigrateListKeyStack            = "stack"
	tfmigrateListKeyTerraformBackend = "terraform_backend"
	tfmigrateListKeyTfmigrateEnabled = "tfmigrate_enabled"
	tfmigrateListKeyWorkspace        = "workspace"
)

type migrateListRenderOptions struct {
	Format    string
	Columns   []string
	Sort      string
	Delimiter string
}

func runTerraformMigrateList(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	if err := migrateListParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	info, err := parseTerraformMigrateListArgs(args)
	if err != nil {
		return err
	}
	shared.ApplyRunOptions(&info, shared.ParseRunOptions(v))

	rows, err := collectTfmigrateListRows(&info)
	if err != nil {
		return err
	}
	return renderTfmigrateList(rows, migrateListRenderOptions{
		Format:    v.GetString(migrateListFlagFormat),
		Columns:   v.GetStringSlice(migrateListFlagColumns),
		Sort:      v.GetString(migrateListFlagSort),
		Delimiter: v.GetString(migrateListFlagDelimiter),
	})
}

func parseTerraformMigrateListArgs(args []string) (schema.ConfigAndStacksInfo, error) {
	info, err := e.ProcessCommandLineArgs("terraform", parentCommand, append([]string{"migrate list"}, args...), compat.GetSeparated())
	if err != nil {
		return schema.ConfigAndStacksInfo{}, err
	}
	if len(args) > 0 {
		info.Components = []string{args[0]}
	}
	return info, nil
}

func collectTfmigrateListRows(info *schema.ConfigAndStacksInfo) ([]map[string]any, error) {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return nil, err
	}
	authManager, err := e.SetupComponentAuthForCLI(&atmosConfig, info)
	if err != nil {
		return nil, err
	}
	stacks, err := e.ExecuteDescribeStacksWithAuthDisabled(
		&atmosConfig,
		info.Stack,
		info.Components,
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
		info.Identity == cfg.IdentityFlagDisabledValue,
	)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	err = walkTfmigrateComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipTfmigrateComponent(componentSection, info.Query) {
			return nil
		}
		rows = append(rows, tfmigrateListRow(stackName, componentName, componentSection))
		return nil
	})
	return rows, err
}

func tfmigrateListRow(stackName, componentName string, componentSection map[string]any) map[string]any {
	backend, _ := componentSection[cfg.BackendSectionName].(map[string]any)
	backendType, _ := componentSection[cfg.BackendTypeSectionName].(string)
	workspace, _ := componentSection[cfg.WorkspaceSectionName].(string)
	history := tfmigrate.HistoryNames(stackName, componentName, workspace)
	backendHistory := tfmigrate.BackendHistoryValues(backendType, backend)
	hook := firstTfmigrateHook(componentSection)

	return map[string]any{
		tfmigrateListKeyComponent:        componentName,
		tfmigrateListKeyStack:            stackName,
		tfmigrateListKeyWorkspace:        history.Workspace,
		tfmigrateListKeyHook:             hook[tfmigrateListKeyName],
		tfmigrateListKeyMode:             hook[tfmigrateListKeyMode],
		tfmigrateListKeyMigration:        hook[tfmigrateListKeyMigration],
		tfmigrateListKeyConfig:           hook[tfmigrateListKeyConfig],
		tfmigrateListKeyHistoryStorage:   backendHistory[tfmigrate.EnvHistoryStorage],
		tfmigrateListKeyHistoryBucket:    backendHistory[tfmigrate.EnvHistoryBucket],
		tfmigrateListKeyHistoryKey:       history.Key,
		tfmigrateListKeyHistoryRoleARN:   backendHistory[tfmigrate.EnvHistoryRoleARN],
		tfmigrateListKeyTerraformBackend: backendType,
		tfmigrateListKeyTfmigrateEnabled: hook[tfmigrateListKeyName] != "",
		tfmigrateListKeyHistoryNamespace: history.Namespace,
		tfmigrateListKeyHistoryRegion:    backendHistory[tfmigrate.EnvHistoryRegion],
		tfmigrateListKeyHistoryProfile:   backendHistory[tfmigrate.EnvHistoryProfile],
		tfmigrateListKeyHistoryEndpoint:  backendHistory[tfmigrate.EnvHistoryEndpoint],
		tfmigrateListKeyHistoryKMSKeyID:  backendHistory[tfmigrate.EnvHistoryKMSKeyID],
	}
}

func firstTfmigrateHook(componentSection map[string]any) map[string]string {
	result := map[string]string{tfmigrateListKeyMode: tfmigrate.ModeDynamic}
	hooksSection, ok := componentSection[cfg.HooksSectionName].(map[string]any)
	if !ok {
		return result
	}
	for name, rawHook := range hooksSection {
		hookConfig, ok := rawHook.(map[string]any)
		if !ok || hookString(hookConfig, tfmigrateListKeyKind) != tfmigrate.Command {
			continue
		}
		result[tfmigrateListKeyName] = name
		result[tfmigrateListKeyMode] = hookStringOrDefault(hookConfig, tfmigrateListKeyMode, tfmigrate.ModeDynamic)
		result[tfmigrateListKeyMigration] = hookString(hookConfig, tfmigrateListKeyMigration)
		result[tfmigrateListKeyConfig] = hookString(hookConfig, tfmigrateListKeyConfig)
		return result
	}
	return result
}

func hookStringOrDefault(hookConfig map[string]any, key string, fallback string) string {
	if value := hookString(hookConfig, key); value != "" {
		return value
	}
	return fallback
}

func hookString(hookConfig map[string]any, key string) string {
	if value, ok := hookConfig[key].(string); ok {
		return value
	}
	return ""
}

func renderTfmigrateList(rows []map[string]any, opts migrateListRenderOptions) error {
	selector, err := column.NewSelector(tfmigrateListColumns(opts.Columns), column.BuildColumnFuncMap())
	if err != nil {
		return err
	}
	sorters, err := listSort.ParseSortSpec(opts.Sort)
	if err != nil {
		return err
	}
	if len(sorters) == 0 {
		sorters = []*listSort.Sorter{
			listSort.NewSorter("Stack", listSort.Ascending),
			listSort.NewSorter("Component", listSort.Ascending),
		}
	}
	r := renderer.New([]listFilter.Filter{}, selector, sorters, listFormat.Format(opts.Format), opts.Delimiter)
	return r.Render(rows)
}

func tfmigrateListColumns(columns []string) []column.Config {
	if len(columns) > 0 {
		configs := make([]column.Config, 0, len(columns))
		for _, spec := range columns {
			if cfg := tfmigrateListColumnSpec(spec); cfg.Name != "" {
				configs = append(configs, cfg)
			}
		}
		if len(configs) > 0 {
			return configs
		}
	}
	return []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Workspace", Value: "{{ .workspace }}"},
		{Name: "Hook", Value: "{{ .hook }}"},
		{Name: "Mode", Value: "{{ .mode }}"},
		{Name: "Migration", Value: "{{ .migration }}"},
		{Name: "Config", Value: "{{ .config }}"},
		{Name: "History Storage", Value: "{{ .history_storage }}"},
		{Name: "History Bucket", Value: "{{ .history_bucket }}"},
		{Name: "History Key", Value: "{{ .history_key }}"},
		{Name: "History Role ARN", Value: "{{ .history_role_arn }}"},
	}
}

func tfmigrateListColumnSpec(spec string) column.Config {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return column.Config{}
	}
	if idx := strings.Index(spec, "="); idx > 0 {
		name := strings.TrimSpace(spec[:idx])
		value := strings.TrimSpace(spec[idx+1:])
		if !strings.Contains(value, "{{") {
			value = "{{ ." + value + " }}"
		}
		return column.Config{Name: name, Value: value}
	}

	return column.Config{
		Name:  strings.Title(spec), //nolint:staticcheck // ASCII CLI column names only.
		Value: "{{ ." + spec + " }}",
	}
}
