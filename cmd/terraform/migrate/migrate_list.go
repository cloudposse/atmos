package migrate

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	errUtils "github.com/cloudposse/atmos/errors"
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

// tfmigrateListRowKeys are exactly the keys tfmigrateListRow populates on
// every row - the valid set for a bare (non "Name=template") --columns entry.
// Deliberately NOT built from every tfmigrateListKey* constant above:
// tfmigrateListKeyKind and tfmigrateListKeyName are used internally (reading
// a hook's own "kind" field, and the intermediate hook-lookup result map in
// firstTfmigrateHook) but are never row keys, so accepting them here would
// silently accept a bare --columns=kind or --columns=name and render the
// Go template engine's "<no value>" placeholder instead of real data.
var tfmigrateListRowKeys = map[string]bool{
	tfmigrateListKeyComponent:        true,
	tfmigrateListKeyStack:            true,
	tfmigrateListKeyWorkspace:        true,
	tfmigrateListKeyHook:             true,
	tfmigrateListKeyMode:             true,
	tfmigrateListKeyMigration:        true,
	tfmigrateListKeyConfig:           true,
	tfmigrateListKeyHistoryStorage:   true,
	tfmigrateListKeyHistoryBucket:    true,
	tfmigrateListKeyHistoryKey:       true,
	tfmigrateListKeyHistoryRoleARN:   true,
	tfmigrateListKeyTerraformBackend: true,
	tfmigrateListKeyTfmigrateEnabled: true,
	tfmigrateListKeyHistoryNamespace: true,
	tfmigrateListKeyHistoryRegion:    true,
	tfmigrateListKeyHistoryProfile:   true,
	tfmigrateListKeyHistoryEndpoint:  true,
	tfmigrateListKeyHistoryKMSKeyID:  true,
}

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
	opts, err := shared.ParseRunOptions(v)
	if err != nil {
		return err
	}
	shared.ApplyRunOptions(&info, opts)
	applyMigrateListComponentArg(&info, args)

	// Resolve the interactive `--identity` sentinel the same way plan/apply do.
	if info.Identity == cfg.IdentityFlagSelectValue {
		if err := shared.HandleInteractiveIdentitySelection(&info); err != nil {
			return err
		}
	}

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
	return info, nil
}

// applyMigrateListComponentArg re-applies the positional component argument as
// a component filter. It must run after shared.ApplyRunOptions, which replaces
// info.Components with the --components flag value.
func applyMigrateListComponentArg(info *schema.ConfigAndStacksInfo, args []string) {
	if len(args) > 0 {
		info.Components = append(info.Components, args[0])
	}
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

// firstTfmigrateHook returns the alphabetically-first (by hook name) kind:
// tfmigrate hook on a component, or an empty result if none is configured.
// "Mode" is only ever set when a matching hook is actually found - callers
// must check "name" (== the Hook column) rather than assume a default mode
// applies when no hook exists.
//
// Sorting hook names before picking one makes the result deterministic: with
// 2+ kind: tfmigrate hooks on one component, iterating componentSection's
// underlying map in Go's randomized order previously picked a different hook
// on every run. Only one hook is ever shown even now - a real limitation for
// multi-hook components, but a documented, stable one instead of a silent
// coin-flip.
func firstTfmigrateHook(componentSection map[string]any) map[string]string {
	result := map[string]string{}
	hooksSection, ok := componentSection[cfg.HooksSectionName].(map[string]any)
	if !ok {
		return result
	}

	names := make([]string, 0, len(hooksSection))
	for name := range hooksSection {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		hookConfig, ok := hooksSection[name].(map[string]any)
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
	columns, err := tfmigrateListColumns(opts.Columns)
	if err != nil {
		return err
	}
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return err
	}
	sorters, err := listSort.ParseSortSpec(opts.Sort)
	if err != nil {
		return err
	}
	if len(sorters) == 0 {
		sorters = defaultTfmigrateListSorters(columns)
	}
	r := renderer.New([]listFilter.Filter{}, selector, sorters, listFormat.Format(opts.Format), opts.Delimiter)
	return r.Render(rows)
}

// defaultTfmigrateListSorters mirrors buildInstanceSorters in
// pkg/list/list_instances.go: only apply the default Stack+Component sort
// when both columns are actually present in the selected column set;
// otherwise skip sorting (natural/insertion order) rather than handing the
// renderer a sorter referencing a column that isn't selected, which the
// renderer's own column-presence check would reject outright ("column
// \"Stack\" not found") even for a perfectly reasonable --columns choice
// like --columns=component,mode.
func defaultTfmigrateListSorters(columns []column.Config) []*listSort.Sorter {
	names := make(map[string]bool, len(columns))
	for _, c := range columns {
		names[c.Name] = true
	}
	if !names["Stack"] || !names["Component"] {
		return nil
	}
	return []*listSort.Sorter{
		listSort.NewSorter("Stack", listSort.Ascending),
		listSort.NewSorter("Component", listSort.Ascending),
	}
}

func tfmigrateListColumns(columns []string) ([]column.Config, error) {
	if len(columns) > 0 {
		configs := make([]column.Config, 0, len(columns))
		for _, spec := range columns {
			cfg, err := tfmigrateListColumnSpec(spec)
			if err != nil {
				return nil, err
			}
			if cfg.Name != "" {
				configs = append(configs, cfg)
			}
		}
		if len(configs) > 0 {
			return configs, nil
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
	}, nil
}

func tfmigrateListColumnSpec(spec string) (column.Config, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return column.Config{}, nil
	}
	if idx := strings.Index(spec, "="); idx > 0 {
		name := strings.TrimSpace(spec[:idx])
		value := strings.TrimSpace(spec[idx+1:])
		if !strings.Contains(value, "{{") {
			value = "{{ ." + value + " }}"
		}
		return column.Config{Name: name, Value: value}, nil
	}

	if !tfmigrateListRowKeys[spec] {
		return column.Config{}, errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanationf("unknown --columns value %q", spec).
			WithContext("column", spec).
			WithHintf("Valid columns: %s", strings.Join(sortedTfmigrateListRowKeys(), ", ")).
			Err()
	}

	return column.Config{
		Name:  strings.Title(spec), //nolint:staticcheck // ASCII CLI column names only.
		Value: "{{ ." + spec + " }}",
	}, nil
}

func sortedTfmigrateListRowKeys() []string {
	keys := make([]string, 0, len(tfmigrateListRowKeys))
	for key := range tfmigrateListRowKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
