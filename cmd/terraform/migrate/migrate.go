package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	errUtils "github.com/cloudposse/atmos/errors"
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
	"github.com/cloudposse/atmos/pkg/ui"
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

	opts, err := shared.ParseRunOptions(v)
	if err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}
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

	if done, err := finalizeTerraformMigrateInfo(cmd, &info); done || err != nil {
		return schema.ConfigAndStacksInfo{}, tfmigrate.Options{}, err
	}

	return info, migrateOpts, nil
}

// finalizeTerraformMigrateInfo runs the shared post-parse steps: argument
// prompts, the help short-circuit (done=true), interactive identity selection,
// and terraform flag validation.
func finalizeTerraformMigrateInfo(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) (bool, error) {
	if err := shared.ResolveAndPromptForArgs(info, cmd); err != nil {
		return false, err
	}
	if info.NeedHelp {
		return true, cmd.Usage()
	}
	if info.Identity == cfg.IdentityFlagSelectValue {
		if err := shared.HandleInteractiveIdentitySelection(info); err != nil {
			return false, err
		}
	}
	return false, shared.CheckTerraformFlags(info)
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

	resolved, err := resolveTfmigrateDefaultConfig(execCtx, opts)
	if resolved.Cleanup != nil {
		defer resolved.Cleanup()
	}
	if err != nil {
		return err
	}
	if resolved.Skip {
		return nil
	}
	opts = resolved.Options

	args, err := tfmigrate.BuildArgs(opts)
	if err != nil {
		return err
	}

	command := execCtx.Toolchain.Resolve(tfmigrate.Command)
	if !execCtx.Info.DryRun {
		// A dry run only prints the command, so it must not require the
		// tfmigrate binary or touch the filesystem.
		if err := tfmigrate.EnsureResolved(command); err != nil {
			return err
		}
		// Pre-create the local history directory (if configured) so tfmigrate's
		// history save doesn't fail after it has already pushed the migrated state.
		if err := tfmigrate.EnsureLocalHistoryDir(execCtx.ComponentDir, opts.Config, execCtx.Env); err != nil {
			return err
		}
	}

	return e.ExecuteShellCommand(
		execCtx.AtmosConfig,
		command,
		args,
		execCtx.ComponentDir,
		execCtx.Env,
		execCtx.Info.DryRun,
		execCtx.Info.RedirectStdErr,
		e.WithEnvironment(execCtx.Info.SanitizedEnv),
	)
}

// tfmigrateDefaultConfigResolution is resolveTfmigrateDefaultConfig's result:
// the (possibly updated) options, an optional cleanup for a generated config
// file, and whether the caller should skip invoking tfmigrate entirely.
type tfmigrateDefaultConfigResolution struct {
	Options tfmigrate.Options
	Cleanup func()
	Skip    bool
}

// resolveTfmigrateDefaultConfig applies Atmos's zero-config tfmigrate default
// when the user has not supplied their own config: it generates one that
// inherits the component's Terraform backend as history storage (so history
// mode works with zero configuration), strips a redundant migration_dir-
// matching prefix from opts.Migration, and reports Skip=true when there is
// nothing to migrate yet (history mode with no migrations/ directory), so the
// caller can return early instead of invoking tfmigrate. The returned cleanup
// func is non-nil only when a config was generated and must be deferred by
// the caller for the lifetime of the tfmigrate invocation, not called here.
func resolveTfmigrateDefaultConfig(execCtx *tfmigrateExecutionContext, opts tfmigrate.Options) (tfmigrateDefaultConfigResolution, error) {
	if opts.Config != "" {
		return tfmigrateDefaultConfigResolution{Options: opts}, nil
	}

	generated, cleanup, err := tfmigrate.EnsureDefaultConfig(&tfmigrate.DefaultConfigInput{
		ComponentDir: execCtx.ComponentDir,
		BackendType:  execCtx.Info.ComponentBackendType,
		Backend:      execCtx.Info.ComponentBackendSection,
		History:      tfmigrate.HistoryNames(execCtx.Info.Stack, tfmigrateComponentName(&execCtx.Info), execCtx.Info.TerraformWorkspace),
	})
	if err != nil {
		return tfmigrateDefaultConfigResolution{}, err
	}
	if generated == "" {
		return tfmigrateDefaultConfigResolution{Options: opts}, nil
	}

	opts.Config = generated
	// Atmos (not the user) controls migration_dir in the config it just
	// generated - strip a redundant migration_dir-matching prefix from a
	// natural-looking --migration path (e.g. "migrations/foo.hcl") before it
	// reaches tfmigrate, which resolves --migration relative to migration_dir
	// itself and would otherwise double-prefix into "migrations/migrations/foo.hcl".
	opts.Migration = tfmigrate.StripMigrationDirPrefix(opts.Migration, execCtx.ComponentDir)

	// History mode (no explicit --migration) with no migrations/ directory
	// means nothing has been authored yet - there is no migration to run.
	// Report that cleanly instead of letting MigrationDirFor's component-root
	// fallback make tfmigrate scan (and fail to parse) Atmos's own generated
	// backend/varfile JSON.
	if tfmigrate.NoMigrationsToRun(opts.Migration, execCtx.ComponentDir) {
		ui.Info(fmt.Sprintf(
			"No tfmigrate migrations found for %s in stack %s - nothing to do. Add a migrations/ directory with a migration file, or pass --migration to run one directly.",
			tfmigrateComponentName(&execCtx.Info), execCtx.Info.Stack,
		))
		return tfmigrateDefaultConfigResolution{Options: opts, Cleanup: cleanup, Skip: true}, nil
	}

	return tfmigrateDefaultConfigResolution{Options: opts, Cleanup: cleanup}, nil
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
	if err := e.ExecuteTerraform(initInfo); err != nil {
		return wrapTfmigrateInitError(err)
	}
	return nil
}

// wrapTfmigrateInitError adds a hint to any failure of Atmos's own
// pre-flight `terraform init` step in the migrate path. The init subprocess
// streams its own stdout/stderr directly to the terminal (already visible to
// the user above this error), so the returned Go error carries no reusable
// text to pattern-match against - a hardcoded substring check against
// OpenTofu/Terraform's exact wording would also be fragile across versions.
// Instead, always point at --skip-init as a next step: it is the fix for the
// specific scenario that motivated this hint (a legacy/unqualified provider
// address in state, which is what `replace-provider` migrations exist to
// fix, and which tfmigrate's own internal init handles gracefully - the
// friction is entirely from this pre-flight step, not tfmigrate itself), and
// is a safe suggestion for other init failures too, since --skip-init is
// specifically designed to let tfmigrate run without Atmos's own init
// succeeding first.
func wrapTfmigrateInitError(err error) error {
	return errUtils.Build(errUtils.ErrInvalidConfig).
		WithCause(err).
		WithExplanation("terraform init failed before tfmigrate could run").
		WithHint("If the state has a legacy/unqualified provider address, add --skip-init - tfmigrate handles Terraform's 0.13 upgrade check internally and ignores that error; Atmos's own pre-flight init does not. A replace-provider migration is typically what fixes the underlying address").
		Err()
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
	// Tfmigrate verifies migrations by running `terraform plan` itself; pass the
	// Atmos-generated varfile through TF_CLI_ARGS_plan so components with
	// required variables plan cleanly. Use an absolute path (not a bare
	// filename) so this still resolves for `migration "multi_state"`, where
	// tfmigrate additionally runs its convergence-check plan from a *second*
	// directory (from_dir), not just this component's own working directory.
	//
	// Only inject -var-file when that file actually exists: --skip-init skips
	// the normal init path (initTfmigrateComponent), which is what generates
	// it, so with --skip-init the file never exists on disk. Terraform's
	// -var-file flag requires the referenced path to exist, so an
	// unconditional injection would fail every --skip-init run with "Given
	// variables file ... does not exist" even for components with no
	// required variables (the exact --skip-init + legacy-provider-address
	// scenario replace-provider migrations need).
	varfilePath, err := e.ConstructTerraformComponentVarfilePath(atmosConfig, info)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(varfilePath); statErr == nil {
		env = tfmigrate.AppendPlanVarFile(env, varfilePath)
	}
	// Secret-bearing and declared-sensitive variables are kept out of the
	// generated varfile; inject them as TF_VAR_* just like the terraform path.
	secretEnv, err := e.ComputeTerraformSecretVarEnv(info)
	if err != nil {
		return nil, err
	}
	env = append(env, secretEnv...)
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
	// Mark this subcommand as experimental.
	migrateCmd.Annotations = map[string]string{"experimental": "true"}

	migrateParser = flags.NewStandardParser(
		shared.WithBackendExecutionFlags(),
		flags.WithStringFlag("migration", "", "", "Path to a single tfmigrate migration file, relative to migration_dir (default: ./migrations if present, else the component root) - do not include that prefix. Omit to let tfmigrate run history mode"),
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

	// No --all flag here: list already shows every component in every stack
	// when no positional [component] arg is given, so --all would have no
	// effect to add - unlike migrate plan/apply, where --all really does
	// select a different (bulk) execution path.
	migrateListParser = flags.NewStandardParser(
		flags.WithStringFlag(migrateListFlagFormat, "f", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringSliceFlag(migrateListFlagColumns, "", nil, "Columns to display"),
		flags.WithStringFlag(migrateListFlagSort, "", "", "Sort by column:order (for example, stack:asc,component:desc)"),
		flags.WithStringFlag(migrateListFlagDelimiter, "", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars(migrateListFlagFormat, "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars(migrateListFlagColumns, "ATMOS_LIST_COLUMNS"),
		flags.WithEnvVars(migrateListFlagSort, "ATMOS_LIST_SORT"),
		flags.WithEnvVars(migrateListFlagDelimiter, "ATMOS_LIST_DELIMITER"),
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
