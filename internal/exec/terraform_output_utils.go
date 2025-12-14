package exec

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	auth_types "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var terraformOutputsCache = sync.Map{}

const (
	dotSeparator             = "."
	cliArgsEnvVar            = "TF_CLI_ARGS"
	inputEnvVar              = "TF_INPUT"
	automationEnvVar         = "TF_IN_AUTOMATION"
	logEnvVar                = "TF_LOG"
	logCoreEnvVar            = "TF_LOG_CORE"
	logPathEnvVar            = "TF_LOG_PATH"
	logProviderEnvVar        = "TF_LOG_PROVIDER"
	reattachEnvVar           = "TF_REATTACH_PROVIDERS"
	appendUserAgentEnvVar    = "TF_APPEND_USER_AGENT"
	workspaceEnvVar          = "TF_WORKSPACE"
	disablePluginTLSEnvVar   = "TF_DISABLE_PLUGIN_TLS"
	skipProviderVerifyEnvVar = "TF_SKIP_PROVIDER_VERIFY"

	varEnvVarPrefix    = "TF_VAR_"
	cliArgEnvVarPrefix = "TF_CLI_ARGS_"
)

var prohibitedEnvVars = []string{
	cliArgsEnvVar,
	inputEnvVar,
	automationEnvVar,
	logEnvVar,
	logCoreEnvVar,
	logPathEnvVar,
	logProviderEnvVar,
	reattachEnvVar,
	appendUserAgentEnvVar,
	workspaceEnvVar,
	disablePluginTLSEnvVar,
	skipProviderVerifyEnvVar,
}

var prohibitedEnvVarPrefixes = []string{
	varEnvVarPrefix,
	cliArgEnvVarPrefix,
}

// authContextWrapper is a minimal AuthManager implementation that only provides
// GetStackInfo() for passing AuthContext to ExecuteDescribeComponent.
// Other methods panic if called since this wrapper is only for propagating existing auth context.
type authContextWrapper struct {
	stackInfo *schema.ConfigAndStacksInfo
}

func (a *authContextWrapper) GetStackInfo() *schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "exec.authContextWrapper.GetStackInfo")()

	return a.stackInfo
}

// Stub methods to satisfy AuthManager interface (not used by ExecuteDescribeComponent).
func (a *authContextWrapper) GetCachedCredentials(ctx context.Context, identityName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetCachedCredentials")()

	panic("authContextWrapper.GetCachedCredentials should not be called")
}

func (a *authContextWrapper) Authenticate(ctx context.Context, identityName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.Authenticate")()

	panic("authContextWrapper.Authenticate should not be called")
}

func (a *authContextWrapper) AuthenticateProvider(ctx context.Context, providerName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.AuthenticateProvider")()

	return nil, fmt.Errorf("%w: authContextWrapper.AuthenticateProvider for template context", errUtils.ErrNotImplemented)
}

func (a *authContextWrapper) Whoami(ctx context.Context, identityName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.Whoami")()

	panic("authContextWrapper.Whoami should not be called")
}

func (a *authContextWrapper) Validate() error {
	defer perf.Track(nil, "exec.authContextWrapper.Validate")()

	panic("authContextWrapper.Validate should not be called")
}

func (a *authContextWrapper) GetDefaultIdentity(forceSelect bool) (string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetDefaultIdentity")()

	panic("authContextWrapper.GetDefaultIdentity should not be called")
}

func (a *authContextWrapper) ListProviders() []string {
	defer perf.Track(nil, "exec.authContextWrapper.ListProviders")()

	panic("authContextWrapper.ListProviders should not be called")
}

func (a *authContextWrapper) Logout(ctx context.Context, identityName string, deleteKeychain bool) error {
	defer perf.Track(nil, "exec.authContextWrapper.Logout")()

	panic("authContextWrapper.Logout should not be called")
}

func (a *authContextWrapper) GetChain() []string {
	defer perf.Track(nil, "exec.authContextWrapper.GetChain")()

	panic("authContextWrapper.GetChain should not be called")
}

func (a *authContextWrapper) ListIdentities() []string {
	defer perf.Track(nil, "exec.authContextWrapper.ListIdentities")()

	panic("authContextWrapper.ListIdentities should not be called")
}

func (a *authContextWrapper) GetProviderForIdentity(identityName string) string {
	defer perf.Track(nil, "exec.authContextWrapper.GetProviderForIdentity")()

	panic("authContextWrapper.GetProviderForIdentity should not be called")
}

func (a *authContextWrapper) GetFilesDisplayPath(providerName string) string {
	defer perf.Track(nil, "exec.authContextWrapper.GetFilesDisplayPath")()

	panic("authContextWrapper.GetFilesDisplayPath should not be called")
}

func (a *authContextWrapper) GetProviderKindForIdentity(identityName string) (string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetProviderKindForIdentity")()

	panic("authContextWrapper.GetProviderKindForIdentity should not be called")
}

func (a *authContextWrapper) GetIdentities() map[string]schema.Identity {
	defer perf.Track(nil, "exec.authContextWrapper.GetIdentities")()

	panic("authContextWrapper.GetIdentities should not be called")
}

func (a *authContextWrapper) GetProviders() map[string]schema.Provider {
	defer perf.Track(nil, "exec.authContextWrapper.GetProviders")()

	panic("authContextWrapper.GetProviders should not be called")
}

func (a *authContextWrapper) LogoutProvider(ctx context.Context, providerName string, deleteKeychain bool) error {
	defer perf.Track(nil, "exec.authContextWrapper.LogoutProvider")()

	panic("authContextWrapper.LogoutProvider should not be called")
}

func (a *authContextWrapper) LogoutAll(ctx context.Context, deleteKeychain bool) error {
	defer perf.Track(nil, "exec.authContextWrapper.LogoutAll")()

	panic("authContextWrapper.LogoutAll should not be called")
}

func (a *authContextWrapper) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetEnvironmentVariables")()

	panic("authContextWrapper.GetEnvironmentVariables should not be called")
}

func (a *authContextWrapper) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.PrepareShellEnvironment")()

	panic("authContextWrapper.PrepareShellEnvironment should not be called")
}

// newAuthContextWrapper creates an AuthManager wrapper that returns the given AuthContext.
func newAuthContextWrapper(authContext *schema.AuthContext) *authContextWrapper {
	if authContext == nil {
		return nil
	}
	return &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: authContext,
		},
	}
}

func execTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	sections map[string]any,
	authContext *schema.AuthContext,
) (map[string]any, error) {
	outputProcessed := map[string]any{}
	componentAbstract := false
	componentEnabled := true
	var err error

	metadataSection, ok := sections[cfg.MetadataSectionName]
	if ok {
		metadata, ok2 := metadataSection.(map[string]any)
		if ok2 {
			componentAbstract = IsComponentAbstract(metadata)
		}
	}

	varsSection, ok := sections[cfg.VarsSectionName]
	if ok {
		vars, ok2 := varsSection.(map[string]any)
		if ok2 {
			componentEnabled = IsComponentEnabled(vars)
		}
	}

	// Don't process Terraform output for disabled and abstract components
	if componentEnabled && !componentAbstract {
		executable, ok := sections[cfg.CommandSectionName].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'command' (executable) defined", component, stack)
		}

		terraformWorkspace, ok := sections[cfg.WorkspaceSectionName].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have Terraform/OpenTofu workspace defined", component, stack)
		}

		componentInfo, ok := sections["component_info"]
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'component_info' defined", component, stack)
		}

		componentInfoMap, ok := componentInfo.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'component_info' section", component, stack)
		}

		componentPath, ok := componentInfoMap["component_path"].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'component_info.component_path' section", component, stack)
		}

		// Auto-generate backend file
		if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
			backendFileName := filepath.Join(componentPath, "backend.tf.json")

			log.Debug("Writing backend config", "file", backendFileName)

			backendTypeSection, ok := sections["backend_type"].(string)
			if !ok {
				return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'backend_type' section", component, stack)
			}

			backendSection, ok := sections["backend"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'backend' section", component, stack)
			}

			componentBackendConfig, err := generateComponentBackendConfig(backendTypeSection, backendSection, terraformWorkspace, authContext)
			if err != nil {
				return nil, err
			}

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0o644)
			if err != nil {
				return nil, err
			}

			log.Debug("Wrote backend config", "file", backendFileName)
		}

		// Generate `providers_override.tf.json` file if the `providers` section is configured
		providersSection, ok := sections[cfg.ProvidersSectionName].(map[string]any)

		if ok && len(providersSection) > 0 {
			providerOverrideFileName := filepath.Join(componentPath, "providers_override.tf.json")

			log.Debug("Writing provider overrides", "file", providerOverrideFileName)

			providerOverrides := generateComponentProviderOverrides(providersSection, authContext)
			err = u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0o644)
			if err != nil {
				return nil, err
			}

			log.Debug("Wrote provider overrides", "file", providerOverrideFileName)
		}

		// Initialize Terraform/OpenTofu
		tf, err := tfexec.NewTerraform(componentPath, executable)
		if err != nil {
			return nil, err
		}

		// Get all environment variables (excluding the variables prohibited by terraform-exec/tfexec) from the parent process.
		environMap := environToMap()

		// Add auth-based environment variables if authContext is provided.
		if authContext != nil && authContext.AWS != nil {
			log.Debug("Adding auth-based environment variables",
				"profile", authContext.AWS.Profile,
				"credentials_file", authContext.AWS.CredentialsFile,
				"config_file", authContext.AWS.ConfigFile,
			)

			// Use shared AWS environment preparation helper.
			// This clears conflicting credential env vars, sets AWS_SHARED_CREDENTIALS_FILE,
			// AWS_CONFIG_FILE, AWS_PROFILE, region, and disables IMDS fallback.
			environMap = awsCloud.PrepareEnvironment(
				environMap,
				authContext.AWS.Profile,
				authContext.AWS.CredentialsFile,
				authContext.AWS.ConfigFile,
				authContext.AWS.Region,
			)
		}

		// Add/override environment variables from the component's 'env' section.
		envSection, ok := sections[cfg.EnvSectionName]
		if ok {
			envMap, ok2 := envSection.(map[string]any)
			if ok2 && len(envMap) > 0 {
				log.Debug("Adding environment variables from component",
					"source", "env section",
					"count", len(envMap),
				)
				for k, v := range envMap {
					environMap[k] = fmt.Sprintf("%v", v)
				}
			}
		}

		// Set the environment variables in the process that executes the `tfexec` functions.
		if len(environMap) > 0 {
			err = tf.SetEnv(environMap)
			if err != nil {
				return nil, err
			}
			log.Debug("Resolved final environment variables",
				"count", len(environMap),
			)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
		defer cancel()

		// 'terraform init'
		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory
		cleanTerraformWorkspace(*atmosConfig, componentPath)

		log.Debug("Executing terraform",
			"command", fmt.Sprintf("terraform init %s -s %s", component, stack),
			cfg.ComponentStr, component,
			cfg.StackStr, stack,
		)

		var initOptions []tfexec.InitOption
		initOptions = append(initOptions, tfexec.Upgrade(false))
		// If `components.terraform.init_run_reconfigure` is set to `true` in atmos.yaml, add the `-reconfigure` flag to `terraform init`
		if atmosConfig.Components.Terraform.InitRunReconfigure {
			initOptions = append(initOptions, tfexec.Reconfigure(true))
		}
		err = tf.Init(ctx, initOptions...)
		if err != nil {
			return nil, err
		}

		log.Debug("Executed terraform",
			"command", fmt.Sprintf("terraform init %s -s %s", component, stack),
			cfg.ComponentStr, component,
			cfg.StackStr, stack,
		)

		// Terraform workspace
		backendType, ok := sections[cfg.BackendTypeSectionName].(string)
		if ok && backendType != "http" {
			log.Debug("Creating a new terraform workspace",
				"command", fmt.Sprintf("terraform workspace new %s", terraformWorkspace),
				cfg.ComponentStr, component,
				cfg.StackStr, stack,
			)
			err = tf.WorkspaceNew(ctx, terraformWorkspace)
			if err != nil {
				log.Debug("Selecting existing terraform workspace",
					"command", fmt.Sprintf("terraform workspace select %s", terraformWorkspace),
					cfg.ComponentStr, component,
					cfg.StackStr, stack,
				)
				err = tf.WorkspaceSelect(ctx, terraformWorkspace)
				if err != nil {
					return nil, err
				}
				log.Debug("Successfully selected terraform workspace",
					"command", fmt.Sprintf("terraform workspace select %s", terraformWorkspace),
					cfg.ComponentStr, component,
					cfg.StackStr, stack,
				)
			} else {
				log.Debug("Successfully created terraform workspace",
					"command", fmt.Sprintf("terraform workspace new %s", terraformWorkspace),
					cfg.ComponentStr, component,
					cfg.StackStr, stack,
				)
			}

			// Add delay on Windows after workspace operations to prevent file locking
			windowsFileDelay()
		}

		// Terraform output
		log.Debug("Executing terraform output command",
			"command", fmt.Sprintf("terraform output %s -s %s", component, stack),
			cfg.ComponentStr, component,
			cfg.StackStr, stack,
		)

		// Add small delay on Windows to prevent file locking issues
		windowsFileDelay()

		// Wrap the output call with retry logic for Windows
		var outputMeta map[string]tfexec.OutputMeta
		err = retryOnWindows(func() error {
			var outputErr error
			outputMeta, outputErr = tf.Output(ctx)
			return outputErr
		})
		if err != nil {
			return nil, err
		}

		log.Debug("Executed terraform output command",
			"command", fmt.Sprintf("terraform output %s -s %s", component, stack),
			cfg.ComponentStr, component,
			cfg.StackStr, stack,
		)

		if atmosConfig.Logs.Level == u.LogLevelTrace {
			y, err2 := u.ConvertToYAML(outputMeta)
			if err2 != nil {
				log.Error("Failed to convert output to YAML", "error", err2)
			} else {
				log.Debug("Raw result of terraform output command",
					"command", fmt.Sprintf("terraform output %s -s %s", component, stack),
					cfg.ComponentStr, component,
					cfg.StackStr, stack,
					"output", y,
				)
			}
		}

		outputProcessed = lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
			s := string(v.Value)

			// Log summary to avoid multiline value formatting issues with concurrent writes.
			valueSummary := s
			if strings.Contains(s, "\n") {
				lineCount := strings.Count(s, "\n") + 1
				valueSummary = fmt.Sprintf("<multiline: %d lines, %d bytes>", lineCount, len(s))
			} else if len(s) > 100 {
				valueSummary = s[:100] + "..."
			}
			log.Debug("Converting variable from JSON to Go data type",
				"variable", k,
				"value_summary", valueSummary,
			)

			d, err2 := u.ConvertFromJSON(s)

			if err2 != nil {
				log.Error("failed to convert output", "output", valueSummary, "error", err2)
				return k, nil
			} else {
				// Log result summary for multiline values.
				resultSummary := fmt.Sprintf("%v", d)
				if strings.Contains(resultSummary, "\n") || len(resultSummary) > 100 {
					resultSummary = fmt.Sprintf("<%T>", d)
				}
				log.Debug("Converted the variable from JSON to Go data type", "key", k, "result_summary", resultSummary)
			}

			return k, d
		})
	} else {
		componentStatus := "disabled"
		if componentAbstract {
			componentStatus = "abstract"
		}
		log.Debug("Skipping terraform output command due to component status",
			"reason", fmt.Sprintf("component is %s", componentStatus),
			"command", fmt.Sprintf("terraform output %s -s %s", component, stack),
			cfg.ComponentStr, component,
			cfg.StackStr, stack,
			"status", componentStatus,
		)
	}

	return outputProcessed, nil
}

// GetTerraformOutput retrieves a specified Terraform output variable for a given component within a stack.
// It optionally uses a cache to avoid redundant state retrievals and supports both static and dynamic backends.
// Parameters:
//   - atmosConfig: Atmos configuration pointer
//   - stack: Stack identifier
//   - component: Component identifier
//   - output: Output variable key to retrieve
//   - skipCache: Flag to bypass cache lookup
//   - authContext: Authentication context for credential access (may be nil)
//   - authManager: Optional auth manager for nested operations that need authentication
//
// Returns:
//   - value: The output value (may be nil if the output exists but has a null value)
//   - exists: Whether the output key exists in the terraform outputs
//
// GetTerraformOutput retrieves the named Terraform output for a specific component in a stack.
// It may return a cached result unless skipCache is true, and it will use the provided authManager
// (if non-nil) or an authContext-derived wrapper to resolve credentials for describing the component.
// If the component is configured to use a static remote state backend, the value is read from that
// static section instead of executing Terraform.
//
// The function returns the output value, a boolean that is true when the output path exists (even if null),
// and an error if retrieval or evaluation failed.
func GetTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "exec.GetTerraformOutput")()

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	if !skipCache {
		cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
		if found && cachedOutputs != nil {
			log.Debug("Cache hit for terraform output",
				"command", fmt.Sprintf("!terraform.output %s %s %s", component, stack, output),
				cfg.ComponentStr, component,
				cfg.StackStr, stack,
				"output", output,
			)
			return getTerraformOutputVariable(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
		}
	}

	message := fmt.Sprintf("Fetching %s output from %s in %s", output, component, stack)

	// Use simple log message in debug/trace mode to avoid concurrent stderr writes with logger.
	// Spinners write to stderr in a separate goroutine, causing misaligned log output.
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		log.Debug(message, "output", output, "component", component, "stack", stack)
	} else {
		// Initialize spinner for normal (non-debug) mode
		p := NewSpinner(message)
		spinnerDone := make(chan struct{})
		// Run spinner in a goroutine
		RunSpinner(p, spinnerDone, message)
		// Ensure the spinner is stopped before returning
		defer StopSpinner(p, spinnerDone)
	}

	// Use the provided authManager directly if available.
	// Otherwise, create an AuthManager wrapper from authContext to pass credentials to ExecuteDescribeComponent.
	// This enables YAML functions within the component config to access remote resources.
	var parentAuthMgr auth.AuthManager
	if authManager != nil {
		// Use the provided authManager (cast from 'any' to auth.AuthManager)
		var ok bool
		parentAuthMgr, ok = authManager.(auth.AuthManager)
		if !ok {
			return nil, false, fmt.Errorf("%w: expected auth.AuthManager", errUtils.ErrInvalidAuthManagerType)
		}
	} else if authContext != nil {
		// Fallback: create wrapper from authContext
		parentAuthMgr = newAuthContextWrapper(authContext)
	}

	// Resolve AuthManager for this nested component.
	// Checks if component has auth config defined:
	//   - If yes: creates component-specific AuthManager with merged auth config
	//   - If no: uses parent AuthManager (inherits authentication)
	// This enables each nested level to optionally override auth settings.
	resolvedAuthMgr, err := resolveAuthManagerForNestedComponent(atmosConfig, component, stack, parentAuthMgr)
	if err != nil {
		log.Debug("Auth does not exist for nested component, using parent AuthManager",
			"component", component,
			"stack", stack,
			"error", err,
		)
		resolvedAuthMgr = parentAuthMgr
	}

	sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          resolvedAuthMgr, // Use resolved AuthManager (may be component-specific or inherited)
	})
	if err != nil {
		u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
		return nil, false, fmt.Errorf("failed to describe the component %s in the stack %s: %w", component, stack, err)
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(&sections)

	var value any
	var exists bool
	var resultErr error

	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformOutputsCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		value, exists, resultErr = GetStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
	} else {
		// Execute `terraform output`
		terraformOutputs, err := execTerraformOutput(atmosConfig, component, stack, sections, authContext)
		if err != nil {
			u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
			return nil, false, fmt.Errorf("failed to execute terraform output for the component %s in the stack %s: %w", component, stack, err)
		}

		// Cache the result
		terraformOutputsCache.Store(stackSlug, terraformOutputs)
		value, exists, resultErr = getTerraformOutputVariable(atmosConfig, component, stack, terraformOutputs, output)
	}

	if resultErr != nil {
		u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
		return nil, false, resultErr
	}

	u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.Checkmark, message)
	return value, exists, nil
}

func getTerraformOutputVariable(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) (any, bool, error) {
	// Use yq to extract the value (handles nested paths, alternative operators, etc.).
	val := output
	if !strings.HasPrefix(output, dotSeparator) {
		val = dotSeparator + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, outputs, val)
	if err != nil {
		return nil, false, fmt.Errorf("failed to evaluate the terraform output for the component %s in the stack %s: %w", component, stack, err)
	}

	// Check if this is a simple key lookup (no yq operators like //, |, etc.).
	// If it's a simple lookup and the key doesn't exist, return not exists.
	// Otherwise trust yq to handle fallback values and complex expressions.
	hasYqOperators := strings.Contains(output, "//") ||
		strings.Contains(output, "|") ||
		strings.Contains(output, "=") ||
		strings.Contains(output, "[") ||
		strings.Contains(output, "]")

	if !hasYqOperators {
		// Simple key lookup - check if key exists.
		outputKey := strings.TrimPrefix(output, dotSeparator)
		// For simple paths without dots, check existence in the map.
		if !strings.Contains(outputKey, dotSeparator) {
			_, exists := outputs[outputKey]
			if !exists {
				return nil, false, nil
			}
		}
		// For nested paths (e.g., "vpc.id"), if res is nil, the path doesn't exist.
		// We can't easily distinguish between missing nested key and null nested value,
		// so we assume if res is nil for a nested path, it exists but is null.
	}

	// Either yq handled the expression (with potential fallback), or key exists.
	// res may be nil if the value is legitimately null.
	return res, true, nil
}

// GetStaticRemoteStateOutput returns static remote state output for a component in a stack.
func GetStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "exec.GetStaticRemoteStateOutput")()

	val := output
	if !strings.HasPrefix(output, dotSeparator) {
		val = dotSeparator + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, remoteStateSection, val)
	if err != nil {
		return nil, false, fmt.Errorf("failed to evaluate the static remote state backend for the component %s in the stack %s: %w", component, stack, err)
	}

	// Check if this is a simple key lookup (no yq operators).
	hasYqOperators := strings.Contains(output, "//") ||
		strings.Contains(output, "|") ||
		strings.Contains(output, "=") ||
		strings.Contains(output, "[") ||
		strings.Contains(output, "]")

	if !hasYqOperators {
		outputKey := strings.TrimPrefix(output, dotSeparator)
		if !strings.Contains(outputKey, dotSeparator) {
			_, exists := remoteStateSection[outputKey]
			if !exists {
				return nil, false, nil
			}
		}
	}

	return res, true, nil
}

// environToMap converts all the environment variables (excluding the variables prohibited by terraform-exec/tfexec) in the environment into a map of strings.
// TODO: review this (find another way to execute `terraform output` not using `terraform-exec/tfexec`).
func environToMap() map[string]string {
	return envpkg.EnvironToMapFiltered(prohibitedEnvVars, prohibitedEnvVarPrefixes)
}
