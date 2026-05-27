package migrate

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	migrateParser     *flags.StandardParser
	migrateListParser *flags.StandardParser
	parentCommand     *cobra.Command
	terraformParser   *flags.StandardParser
)

// Options contains dependencies supplied by the parent terraform command package.
type Options struct {
	ParentCommand   *cobra.Command
	TerraformParser *flags.StandardParser
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run Terraform state migrations with tfmigrate",
	Long: `Run tfmigrate in an Atmos Terraform component context.

Atmos prepares the component the same way it does for Terraform operations: it resolves
the stack/component, authenticates, generates backend and variable files, initializes the
working directory unless skipped, selects the Terraform workspace, and then runs tfmigrate.`,
}

var migratePlanCmd = newMigrateActionCmd(
	tfmigrate.ActionPlan,
	"Compute a migrated Terraform state with tfmigrate",
)

var migrateApplyCmd = newMigrateActionCmd(
	tfmigrate.ActionApply,
	"Apply a migrated Terraform state with tfmigrate",
)

var migrateListCmd = &cobra.Command{
	Use:   "list [component]",
	Short: "List Terraform component instances with tfmigrate context",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTerraformMigrateList,
}

type tfmigrateExecutionContext struct {
	AtmosConfig  schema.AtmosConfiguration
	Info         schema.ConfigAndStacksInfo
	Toolchain    *dependencies.ToolchainEnvironment
	ComponentDir string
	Env          []string
}

func newMigrateActionCmd(action, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   action + " [component]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTerraformMigrate(cmd, args, action)
		},
	}
	return cmd
}

func runTerraformMigrate(cmd *cobra.Command, args []string, action string) error {
	info, migrateOpts, err := parseTerraformMigrate(cmd, args, action)
	if err != nil {
		return err
	}

	if info.Affected {
		return executeAffectedMigrateCommand(cmd, args, &info, migrateOpts)
	}
	if info.All || shared.IsMultiComponentExecution(&info) {
		return executeTfmigrateQuery(&info, migrateOpts)
	}
	return executeTfmigrateSingle(&info, migrateOpts)
}

func parseTerraformMigrate(cmd *cobra.Command, args []string, action string) (schema.ConfigAndStacksInfo, tfmigrate.Options, error) {
	v := viper.GetViper()
	if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}
	if err := migrateParser.BindFlagsToViper(cmd, v); err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}

	opts := shared.ParseRunOptions(v)
	migrateOpts := tfmigrate.Options{
		Action:        action,
		Migration:     v.GetString("migration"),
		Config:        v.GetString("tfmigrate-config"),
		BackendConfig: v.GetStringSlice("backend-config"),
	}
	if err := migrateOpts.Validate(); err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}

	argsWithSubCommand := append([]string{"migrate " + action}, args...)
	info, err := e.ProcessCommandLineArgs("terraform", parentCommand, argsWithSubCommand, compat.GetSeparated())
	if err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}
	shared.ApplyRunOptions(&info, opts)

	if err := shared.ResolveAndPromptForArgs(&info, cmd); err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}
	if info.NeedHelp {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, cmd.Usage()
	}
	if info.Identity == cfg.IdentityFlagSelectValue {
		if err := shared.HandleInteractiveIdentitySelection(&info); err != nil {
			return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
		}
	}
	if err := shared.CheckTerraformFlags(&info); err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}

	return info, migrateOpts, nil
}

func executeTfmigrateSingle(info *schema.ConfigAndStacksInfo, opts tfmigrate.Options) error {
	defer perf.Track(nil, "terraform.executeTfmigrateSingle")()

	execCtx, err := prepareTfmigrateExecution(info)
	if err != nil {
		return err
	}

	if err := selectTfmigrateWorkspace(execCtx); err != nil {
		return err
	}

	args, err := tfmigrate.BuildArgs(opts)
	if err != nil {
		return err
	}

	return e.ExecuteShellCommand(
		execCtx.AtmosConfig,
		execCtx.Toolchain.Resolve(tfmigrate.Command),
		args,
		execCtx.ComponentDir,
		execCtx.Env,
		execCtx.Info.DryRun,
		execCtx.Info.RedirectStdErr,
		e.WithEnvironment(execCtx.Info.SanitizedEnv),
	)
}

func prepareTfmigrateExecution(info *schema.ConfigAndStacksInfo) (*tfmigrateExecutionContext, error) {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return nil, err
	}

	authManager, err := e.SetupComponentAuthForCLI(&atmosConfig, info)
	if err != nil {
		return nil, err
	}

	processedInfo, err := e.ProcessStacks(&atmosConfig, *info, true, info.ProcessTemplates, info.ProcessFunctions, info.Skip, authManager)
	if err != nil {
		return nil, err
	}
	info = &processedInfo

	setTfmigrateTerraformCommand(info, &atmosConfig)

	if err := auth.TerraformPreHook(&atmosConfig, info); err != nil {
		return nil, err
	}

	if err := initTfmigrateComponent(info); err != nil {
		return nil, err
	}

	componentPath, err := resolveTfmigrateComponentPath(&atmosConfig, info)
	if err != nil {
		return nil, err
	}

	tenv, err := dependencies.ForComponent(&atmosConfig, cfg.TerraformComponentType, info.StackSection, info.ComponentSection)
	if err != nil {
		return nil, err
	}

	env, err := buildTfmigrateEnv(&atmosConfig, info, tenv)
	if err != nil {
		return nil, err
	}

	return &tfmigrateExecutionContext{
		AtmosConfig:  atmosConfig,
		Info:         *info,
		Toolchain:    tenv,
		ComponentDir: componentPath,
		Env:          env,
	}, nil
}

func setTfmigrateTerraformCommand(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) {
	if info.Command == "" {
		info.Command = atmosConfig.Components.Terraform.Command
	}
	if info.Command == "" {
		info.Command = cfg.TerraformComponentType
	}
}

func initTfmigrateComponent(info *schema.ConfigAndStacksInfo) error {
	// Run the normal init path first. This is what guarantees source/workdir
	// provisioning and generated backend/var files happen before tfmigrate.
	if info.SkipInit {
		return nil
	}
	initInfo := *info
	initInfo.SubCommand = "init"
	return e.ExecuteTerraform(initInfo)
}

func selectTfmigrateWorkspace(execCtx *tfmigrateExecutionContext) error {
	if execCtx.Info.TerraformWorkspace == "" {
		return nil
	}

	return e.ExecuteShellCommand(
		execCtx.AtmosConfig,
		execCtx.Toolchain.Resolve(execCtx.Info.Command),
		[]string{"workspace", "select", execCtx.Info.TerraformWorkspace},
		execCtx.ComponentDir,
		execCtx.Env,
		execCtx.Info.DryRun,
		execCtx.Info.RedirectStdErr,
		e.WithEnvironment(execCtx.Info.SanitizedEnv),
	)
}

func resolveTfmigrateComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	basePath, err := u.GetComponentPath(atmosConfig, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", err
	}

	if path, exists, err := component.BuildAndResolveWorkdirPath(atmosConfig, info, cfg.TerraformComponentType); err == nil && exists && path != "" {
		return path, nil
	} else if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	path, _, err := component.ProvisionAndResolveComponentPath(ctx, atmosConfig, info, cfg.TerraformComponentType, basePath)
	if err != nil {
		return "", err
	}
	return path, nil
}

func buildTfmigrateEnv(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, tenv *dependencies.ToolchainEnvironment) ([]string, error) {
	env := make([]string, 0, len(info.ComponentEnvSection)+22)
	for k, v := range info.ComponentEnvSection {
		env = append(env, fmt.Sprintf("%s=%v", k, v))
	}
	env = append(env, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return nil, err
	}
	env = append(env, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	env = append(env, "TF_IN_AUTOMATION=true")
	env = append(env, tfmigrate.HistoryEnv(info.Stack, tfmigrateComponentName(info), info.TerraformWorkspace)...)
	env = append(env, tfmigrate.BackendHistoryEnv(info.ComponentBackendType, info.ComponentBackendSection)...)
	terraformCommand := info.Command
	if tenv != nil {
		env = append(env, tenv.EnvVars()...)
		terraformCommand = tenv.Resolve(info.Command)
	}
	env = tfmigrate.AppendExecPath(env, terraformCommand)
	return env, nil
}

func tfmigrateComponentName(info *schema.ConfigAndStacksInfo) string {
	switch {
	case info.FinalComponent != "":
		return info.FinalComponent
	case info.ComponentFromArg != "":
		return info.ComponentFromArg
	default:
		return info.Component
	}
}

func init() {
	migrateParser = flags.NewStandardParser(
		shared.WithBackendExecutionFlags(),
		flags.WithStringFlag("migration", "", "", "Path to a single tfmigrate migration file. Omit to let tfmigrate run history mode"),
		flags.WithStringFlag("tfmigrate-config", "", "", "Override tfmigrate config path. Defaults to tfmigrate discovery"),
		flags.WithStringSliceFlag("backend-config", "", nil, "Backend configuration passed to tfmigrate; may be specified multiple times"),
		flags.WithBoolFlag("affected", "", false, "Run migrations for the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Run migrations for all components in all stacks"),
		flags.WithEnvVars("migration", "ATMOS_TFMIGRATE_MIGRATION"),
		flags.WithEnvVars("tfmigrate-config", "ATMOS_TFMIGRATE_CONFIG"),
		flags.WithEnvVars("backend-config", "ATMOS_TFMIGRATE_BACKEND_CONFIG"),
	)

	migrateParser.RegisterFlags(migratePlanCmd)
	migrateParser.RegisterFlags(migrateApplyCmd)
	if err := migrateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	migrateListParser = flags.NewStandardParser(
		flags.WithStringFlag(migrateListFlagFormat, "f", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringSliceFlag(migrateListFlagColumns, "", nil, "Columns to display"),
		flags.WithStringFlag(migrateListFlagSort, "", "", "Sort by column:order (for example, stack:asc,component:desc)"),
		flags.WithStringFlag(migrateListFlagDelimiter, "", "", "Delimiter for CSV/TSV output"),
		flags.WithBoolFlag("all", "", false, "List tfmigrate context for all components in all stacks"),
		flags.WithEnvVars(migrateListFlagFormat, "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars(migrateListFlagColumns, "ATMOS_LIST_COLUMNS"),
		flags.WithEnvVars(migrateListFlagSort, "ATMOS_LIST_SORT"),
		flags.WithEnvVars(migrateListFlagDelimiter, "ATMOS_LIST_DELIMITER"),
		flags.WithEnvVars("all", "ATMOS_ALL"),
	)
	migrateListParser.RegisterFlags(migrateListCmd)
	if err := migrateListParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	shared.RegisterCompletions(migratePlanCmd)
	shared.RegisterCompletions(migrateApplyCmd)
	shared.RegisterCompletions(migrateListCmd)

	migrateCmd.AddCommand(migratePlanCmd, migrateApplyCmd, migrateListCmd)
}

// GetCommand returns the migrate command for parent registration.
func GetCommand(opts Options) *cobra.Command {
	parentCommand = opts.ParentCommand
	terraformParser = opts.TerraformParser
	return migrateCmd
}

// CompatFlags returns compatibility flags for the atmos terraform migrate command.
func CompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{}
}
