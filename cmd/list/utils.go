package list

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	f "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// checkAtmosConfig verifies that Atmos is properly configured.
// Returns an error instead of calling Exit to allow proper error handling in tests.
// The cmd and v parameters allow honoring config selection flags (--base-path, --config, --config-path, --profile).
func checkAtmosConfig(cmd *cobra.Command, v *viper.Viper, skipStackCheck ...bool) error {
	// Parse global flags and build ConfigAndStacksInfo to honor config selection flags.
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	configAndStacksInfo := buildConfigAndStacksInfo(&globalFlags)

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	// Allow skipping stack validation for commands that don't need it (e.g., workflows).
	if len(skipStackCheck) > 0 && skipStackCheck[0] {
		return nil
	}

	atmosConfigExists, err := u.IsDirectory(atmosConfig.StacksBaseAbsolutePath)
	if !atmosConfigExists || err != nil {
		return fmt.Errorf("atmos stacks directory not found at: %s", filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath))
	}

	return nil
}

// buildConfigAndStacksInfo creates a ConfigAndStacksInfo struct from global flags.
// This ensures that config selection flags (--base-path, --config, --config-path, --profile)
// are properly honored when initializing CLI config.
func buildConfigAndStacksInfo(globalFlags *global.Flags) schema.ConfigAndStacksInfo {
	if globalFlags == nil {
		return schema.ConfigAndStacksInfo{}
	}
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

// addStackCompletion adds the --stack flag with shell completion to a command.
func addStackCompletion(cobraCmd *cobra.Command) {
	if cobraCmd.Flag("stack") == nil {
		cobraCmd.PersistentFlags().StringP("stack", "s", "", "Filter by stack name or pattern")
	}
	cobraCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)
}

// stackFlagCompletion provides shell completion for the --stack flag.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksForComponent(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks
	output, err := listAllStacks()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listStacksForComponent returns stacks that contain the specified component.
func listStacksForComponent(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, component)
	return output, err
}

// listAllStacks returns all available stacks.
func listAllStacks() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %v", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, "")
	return output, err
}

// newCommonListParser creates a StandardParser with common list flags.
// This replaces the pkg/list/flags.AddCommonListFlags pattern.
func newCommonListParser(additionalOptions ...flags.Option) *flags.StandardParser {
	// Start with common list flags
	options := []flags.Option{
		flags.WithStringFlag("format", "", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithIntFlag("max-columns", "", 0, "Maximum number of columns to display"),
		flags.WithStringFlag("delimiter", "", "", "Delimiter for CSV/TSV output"),
		flags.WithStringFlag("stack", "s", "", "Stack pattern to filter by"),
		flags.WithStringFlag("query", "", "", "YQ expression to filter values (e.g., .vars.region)"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars("delimiter", "ATMOS_LIST_DELIMITER"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("query", "ATMOS_LIST_QUERY"),
	}

	// Append any additional flags
	options = append(options, additionalOptions...)

	return flags.NewStandardParser(options...)
}

// getIdentityFromCommand gets the identity value from --identity flag or ATMOS_IDENTITY env var.
// The --identity flag is inherited from the parent list command.
// Returns empty string if no identity is specified.
//
// Note: This is a simplified version of cmd.GetIdentityFromFlags that doesn't need
// to handle the NoOptDefVal quirk because list commands use persistent flags.
// We can't import cmd.GetIdentityFromFlags due to import cycle (cmd imports cmd/list).
func getIdentityFromCommand(cmd *cobra.Command) string {
	var value string

	if flag := lookupChangedIdentityFlag(cmd); flag != nil {
		value = flag.Value.String()
		return normalizeIdentityValue(value)
	}

	// Fall back to environment variable via Viper.
	value = viper.GetString(cfg.IdentityFlagName)
	return normalizeIdentityValue(value)
}

func lookupChangedIdentityFlag(cmd *cobra.Command) *pflag.Flag {
	for current := cmd; current != nil; current = current.Parent() {
		for _, flagSet := range []*pflag.FlagSet{
			current.Flags(),
			current.InheritedFlags(),
			current.PersistentFlags(),
		} {
			if flag := flagSet.Lookup(cfg.IdentityFlagName); flag != nil && flag.Changed {
				return flag
			}
		}
	}
	return nil
}

// normalizeIdentityValue converts boolean false representations to the disabled sentinel value.
// Recognizes: false, False, FALSE, 0, no, No, NO, off, Off, OFF.
// All other values are returned unchanged.
//
// Deprecated: Use cfg.NormalizeIdentityValue() instead. This wrapper exists for backward compatibility.
func normalizeIdentityValue(value string) string {
	return cfg.NormalizeIdentityValue(value)
}

// AuthManagerFactory abstracts auth.CreateAndAuthenticateManagerWithStackScan so
// createAuthManagerForList's evaluation policy can be verified without performing real
// authentication.
type AuthManagerFactory interface {
	// CreateWithStackScan creates and authenticates an AuthManager, first running the stack-file
	// pre-scan to discover stack-level default identities.
	CreateWithStackScan(
		identity string,
		authConfig *schema.AuthConfig,
		selectValue string,
		atmosConfig *schema.AtmosConfiguration,
	) (auth.AuthManager, error)
}

// defaultAuthManagerFactory implements AuthManagerFactory using pkg/auth.
type defaultAuthManagerFactory struct{}

func (defaultAuthManagerFactory) CreateWithStackScan(
	identity string,
	authConfig *schema.AuthConfig,
	selectValue string,
	atmosConfig *schema.AtmosConfiguration,
) (auth.AuthManager, error) {
	return auth.CreateAndAuthenticateManagerWithStackScan(identity, authConfig, selectValue, atmosConfig)
}

// listAuthManagerFactory is replaceable in tests so command-level auth policy can be verified
// without performing real authentication.
var listAuthManagerFactory AuthManagerFactory = defaultAuthManagerFactory{}

// createAuthManagerForList creates an AuthManager when the command will evaluate values
// that can require credentials, or when the caller explicitly selected an identity. Plain
// inventory runs with both template and YAML-function processing disabled remain credential-free.
// An explicit --identity=false always disables authentication.
func createAuthManagerForList(
	cmd *cobra.Command,
	atmosConfig *schema.AtmosConfiguration,
	processTemplates, processYamlFunctions bool,
) (auth.AuthManager, error) {
	identityName := getIdentityFromCommand(cmd)
	if identityName == cfg.IdentityFlagDisabledValue {
		return nil, nil
	}
	if identityName == "" && !processTemplates && !processYamlFunctions {
		return nil, nil
	}

	authManager, err := listAuthManagerFactory.CreateWithStackScan(
		identityName,
		&atmosConfig.Auth,
		cfg.IdentityFlagSelectValue,
		atmosConfig,
	)
	if err != nil {
		return nil, err
	}

	return authManager, nil
}

// credentialBackedYAMLFunctions returns every Atmos YAML function that can only be
// evaluated by contacting a cloud/backend API with live credentials. The names are
// bare (no leading `!`) to match `skipFunc`, which trims the tag prefix before comparing.
func credentialBackedYAMLFunctions() []string {
	return []string{
		strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncStore, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncStoreGet, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncSecret, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncAwsAccountID, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncAwsCallerIdentityArn, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncAwsCallerIdentityUserID, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncAwsRegion, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncAwsOrganizationID, "!"),
	}
}

// skipCredentialBackedYAMLFunctionsForInventory returns the caller's `--skip` list plus
// every credential-backed YAML function.
//
// Inventory commands (`list stacks`, `list components`, `list dependencies`,
// `list instances`) enumerate the whole repository: they walk every stack in every
// account so they can report which stacks/components exist. Their output is the
// inventory itself, never a resolved `!terraform.state` / `!store` / `!aws.*` value, so
// evaluating those functions is pure overhead — and in a multi-account repository it is
// a hard failure, because no single set of credentials can read every account's state
// backend. Authenticating does not change that: an identity covers one account while the
// scan still spans all of them, which is exactly how #2566 reproduced
// (`atmos list stacks --identity prd-…` aborting on an `AccessDenied` reading the `dev-…`
// state bucket, and vice versa).
//
// The skip used to be conditional on the AuthManager being nil, which meant it stopped
// applying the moment credentials existed — precisely backwards. Since #2801,
// createAuthManagerForList also returns a manager whenever templates or YAML functions are
// processed (both default to true), so that condition made a plain `atmos list stacks` — no
// `--identity` at all — attempt cross-account state reads.
//
// The skip is therefore unconditional. Use `atmos describe stacks`/`describe component`
// when you need resolved values for a scope you actually hold credentials for.
//
// Note this covers YAML functions only. Go templates calling `atmos.Component(...)` can
// still perform authenticated cross-account reads; #2801 deliberately restored auth for
// that path, and `--process-templates=false` remains the way to opt out of it.
func skipCredentialBackedYAMLFunctionsForInventory(skip []string) []string {
	merged := append([]string{}, skip...)
	for _, name := range credentialBackedYAMLFunctions() {
		if !containsString(merged, name) {
			merged = append(merged, name)
		}
	}
	return merged
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// setDefaultCSVDelimiter sets the delimiter to comma if CSV format is used and delimiter is default TSV.
func setDefaultCSVDelimiter(delimiter *string, format string) {
	if f.Format(format) == f.FormatCSV && *delimiter == f.DefaultTSVDelimiter {
		*delimiter = f.DefaultCSVDelimiter
	}
}

// getComponentFilter extracts the component filter from command arguments.
func getComponentFilter(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// initConfigAndAuth initializes CLI config and creates an auth manager.
// The cmd and v parameters allow honoring config selection flags (--base-path, --config, --config-path, --profile).
func initConfigAndAuth(
	cmd *cobra.Command,
	v *viper.Viper,
	processTemplates, processYamlFunctions bool,
) (schema.AtmosConfiguration, auth.AuthManager, error) {
	// Parse global flags and build ConfigAndStacksInfo to honor config selection flags.
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	configAndStacksInfo := buildConfigAndStacksInfo(&globalFlags)
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, &listerrors.InitConfigError{Cause: err}
	}

	authManager, err := createAuthManagerForList(cmd, &atmosConfig, processTemplates, processYamlFunctions)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, err
	}

	return atmosConfig, authManager, nil
}

// validateComponentFilter validates that the component exists if a filter is specified.
func validateComponentFilter(atmosConfig *schema.AtmosConfiguration, componentFilter string) error {
	if componentFilter != "" && !listutils.CheckComponentExists(atmosConfig, componentFilter) {
		return &listerrors.ComponentDefinitionNotFoundError{Component: componentFilter}
	}
	return nil
}

// handleNoValuesError handles the NoValuesFoundError by logging an appropriate message.
// LogFunc is called with the componentFilter when no values are found.
func handleNoValuesError(err error, componentFilter string, logFunc func(string)) (string, error) {
	var noValuesErr *listerrors.NoValuesFoundError
	if errors.As(err, &noValuesErr) {
		logFunc(componentFilter)
		return "", nil
	}
	return "", err
}

// renderWithPager renders data using the renderer, optionally using a pager for interactive display.
// If pager is enabled in atmosConfig and TTY is available, the output is displayed in a scrollable pager.
// Otherwise, the output is written directly to stdout.
func renderWithPager(atmosConfig *schema.AtmosConfiguration, title string, r *renderer.Renderer, data []map[string]any) error {
	defer perf.Track(atmosConfig, "list.renderWithPager")()

	// Check if pager is enabled in config.
	if atmosConfig.Settings.Terminal.IsPagerEnabled() {
		// Get rendered content as string.
		content, err := r.RenderToString(data)
		if err != nil {
			return err
		}

		// Try to use pager - it handles TTY detection and falls back to direct print.
		pageCreator := pager.NewWithAtmosConfig(true, atmosConfig.Settings.Terminal.Speed)
		if err := pageCreator.Run(title, content); err != nil {
			// Pager failed, fall back to direct render.
			return r.Render(data)
		}
		return nil
	}

	// No pager - render directly.
	return r.Render(data)
}
