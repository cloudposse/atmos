package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// FindAllStackConfigsInPathsForStack finds all stack manifests in the paths specified by globs for the provided stack
func FindAllStackConfigsInPathsForStack(
	cliConfig schema.CliConfiguration,
	stack string,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, bool, error) {

	var absolutePaths []string
	var relativePaths []string
	var stackIsDir = strings.IndexAny(stack, "/") > 0

	for _, p := range includeStackPaths {
		// Try both regular and template patterns
		patterns := []string{p}

		ext := filepath.Ext(p)
		if ext == "" {
			// Get all patterns since filtering is done later
			patterns = getStackFilePatterns(p, true)
		}

		var allMatches []string
		for _, pattern := range patterns {
			// Find all matches in the glob
			matches, err := u.GetGlobMatches(pattern)
			if err == nil && len(matches) > 0 {
				allMatches = append(allMatches, matches...)
			}
		}

		// If no matches were found across all patterns, we perform an additional check:
		// We try to get matches for the first pattern only to determine if there's a genuine error
		// (like permission issues or invalid path) versus simply no matching files.
		if len(allMatches) == 0 {
			_, err := u.GetGlobMatches(patterns[0])
			if err != nil {
				if cliConfig.Logs.Level == u.LogLevelTrace {
					y, _ := u.ConvertToYAML(cliConfig)
					return nil, nil, false, fmt.Errorf("%v\n\n\nCLI config:\n\n%v", err, y)
				}
				return nil, nil, false, err
			}
			// If there's no error but still no matches, we continue to the next path
			// This happens when the pattern is valid but no files match it
			continue
		}

		// Process all matches found
		for _, matchedFileAbsolutePath := range allMatches {
			matchedFileRelativePath := u.TrimBasePathFromPath(cliConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)

			// Check if the provided stack matches a file in the config folders (excluding the files from `excludeStackPaths`)
			stackMatch := matchesStackFilePattern(matchedFileAbsolutePath, stack)

			if stackMatch {
				allExcluded := true
				for _, excludePath := range excludeStackPaths {
					excludeMatch, err := u.PathMatch(excludePath, matchedFileAbsolutePath)
					if err != nil {
						continue
					} else if excludeMatch {
						allExcluded = false
						break
					}
				}

				if allExcluded && stackIsDir {
					return []string{matchedFileAbsolutePath}, []string{matchedFileRelativePath}, true, nil
				}
			}

			include := true

			for _, excludePath := range excludeStackPaths {
				excludeMatch, err := u.PathMatch(excludePath, matchedFileAbsolutePath)
				if err != nil {
					include = false
					continue
				} else if excludeMatch {
					include = false
					continue
				}
			}

			if include {
				absolutePaths = append(absolutePaths, matchedFileAbsolutePath)
				relativePaths = append(relativePaths, matchedFileRelativePath)
			}
		}
	}

	if len(absolutePaths) == 0 {
		return nil, nil, false, fmt.Errorf("no matches found for the provided stack '%s' in the paths %v", stack, includeStackPaths)
	}

	return absolutePaths, relativePaths, false, nil
}

// FindAllStackConfigsInPaths finds all stack manifests in the paths specified by globs
func FindAllStackConfigsInPaths(
	cliConfig schema.CliConfiguration,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, error) {

	var absolutePaths []string
	var relativePaths []string

	for _, p := range includeStackPaths {
		patterns := []string{p}

		ext := filepath.Ext(p)
		if ext == "" {
			// Get all patterns since filtering is done later
			patterns = getStackFilePatterns(p, true)
		}

		var allMatches []string
		for _, pattern := range patterns {
			// Find all matches in the glob
			matches, err := u.GetGlobMatches(pattern)
			if err == nil && len(matches) > 0 {
				allMatches = append(allMatches, matches...)
			}
		}

		// If no matches were found across all patterns, we perform an additional check:
		// We try to get matches for the first pattern only to determine if there's a genuine error
		// (like permission issues or invalid path) versus simply no matching files.
		if len(allMatches) == 0 {
			_, err := u.GetGlobMatches(patterns[0])
			if err != nil {
				if cliConfig.Logs.Level == u.LogLevelTrace {
					y, _ := u.ConvertToYAML(cliConfig)
					return nil, nil, fmt.Errorf("%v\n\n\nCLI config:\n\n%v", err, y)
				}
				return nil, nil, err
			}
			// If there's no error but still no matches, we continue to the next path
			// This happens when the pattern is valid but no files match it
			continue
		}

		// Process all matches found
		for _, matchedFileAbsolutePath := range allMatches {
			matchedFileRelativePath := u.TrimBasePathFromPath(cliConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)
			include := true

			for _, excludePath := range excludeStackPaths {
				excludeMatch, err := u.PathMatch(excludePath, matchedFileAbsolutePath)
				if err != nil {
					include = false
					continue
				} else if excludeMatch {
					include = false
					continue
				}
			}

			if include {
				absolutePaths = append(absolutePaths, matchedFileAbsolutePath)
				relativePaths = append(relativePaths, matchedFileRelativePath)
			}
		}
	}

	return absolutePaths, relativePaths, nil
}

func processEnvVars(cliConfig *schema.CliConfiguration) error {
	basePath := os.Getenv("ATMOS_BASE_PATH")
	if len(basePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_BASE_PATH=%s", basePath))
		cliConfig.BasePath = basePath
	}

	vendorBasePath := os.Getenv("ATMOS_VENDOR_BASE_PATH")
	if len(vendorBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_VENDOR_BASE_PATH=%s", vendorBasePath))
		cliConfig.Vendor.BasePath = vendorBasePath
	}

	stacksBasePath := os.Getenv("ATMOS_STACKS_BASE_PATH")
	if len(stacksBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_STACKS_BASE_PATH=%s", stacksBasePath))
		cliConfig.Stacks.BasePath = stacksBasePath
	}

	stacksIncludedPaths := os.Getenv("ATMOS_STACKS_INCLUDED_PATHS")
	if len(stacksIncludedPaths) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_STACKS_INCLUDED_PATHS=%s", stacksIncludedPaths))
		cliConfig.Stacks.IncludedPaths = strings.Split(stacksIncludedPaths, ",")
	}

	stacksExcludedPaths := os.Getenv("ATMOS_STACKS_EXCLUDED_PATHS")
	if len(stacksExcludedPaths) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_STACKS_EXCLUDED_PATHS=%s", stacksExcludedPaths))
		cliConfig.Stacks.ExcludedPaths = strings.Split(stacksExcludedPaths, ",")
	}

	stacksNamePattern := os.Getenv("ATMOS_STACKS_NAME_PATTERN")
	if len(stacksNamePattern) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_STACKS_NAME_PATTERN=%s", stacksNamePattern))
		cliConfig.Stacks.NamePattern = stacksNamePattern
	}

	stacksNameTemplate := os.Getenv("ATMOS_STACKS_NAME_TEMPLATE")
	if len(stacksNameTemplate) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_STACKS_NAME_TEMPLATE=%s", stacksNameTemplate))
		cliConfig.Stacks.NameTemplate = stacksNameTemplate
	}

	componentsTerraformCommand := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND")
	if len(componentsTerraformCommand) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_COMMAND=%s", componentsTerraformCommand))
		cliConfig.Components.Terraform.Command = componentsTerraformCommand
	}

	componentsTerraformBasePath := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH")
	if len(componentsTerraformBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_BASE_PATH=%s", componentsTerraformBasePath))
		cliConfig.Components.Terraform.BasePath = componentsTerraformBasePath
	}

	componentsTerraformApplyAutoApprove := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE")
	if len(componentsTerraformApplyAutoApprove) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=%s", componentsTerraformApplyAutoApprove))
		applyAutoApproveBool, err := strconv.ParseBool(componentsTerraformApplyAutoApprove)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.ApplyAutoApprove = applyAutoApproveBool
	}

	componentsTerraformDeployRunInit := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT")
	if len(componentsTerraformDeployRunInit) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT=%s", componentsTerraformDeployRunInit))
		deployRunInitBool, err := strconv.ParseBool(componentsTerraformDeployRunInit)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.DeployRunInit = deployRunInitBool
	}

	componentsInitRunReconfigure := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE")
	if len(componentsInitRunReconfigure) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE=%s", componentsInitRunReconfigure))
		initRunReconfigureBool, err := strconv.ParseBool(componentsInitRunReconfigure)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.InitRunReconfigure = initRunReconfigureBool
	}

	componentsTerraformAutoGenerateBackendFile := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE")
	if len(componentsTerraformAutoGenerateBackendFile) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE=%s", componentsTerraformAutoGenerateBackendFile))
		componentsTerraformAutoGenerateBackendFileBool, err := strconv.ParseBool(componentsTerraformAutoGenerateBackendFile)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.AutoGenerateBackendFile = componentsTerraformAutoGenerateBackendFileBool
	}

	componentsHelmfileCommand := os.Getenv("ATMOS_COMPONENTS_HELMFILE_COMMAND")
	if len(componentsHelmfileCommand) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_COMMAND=%s", componentsHelmfileCommand))
		cliConfig.Components.Helmfile.Command = componentsHelmfileCommand
	}

	componentsHelmfileBasePath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH")
	if len(componentsHelmfileBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_BASE_PATH=%s", componentsHelmfileBasePath))
		cliConfig.Components.Helmfile.BasePath = componentsHelmfileBasePath
	}

	componentsHelmfileUseEKS := os.Getenv("ATMOS_COMPONENTS_HELMFILE_USE_EKS")
	if len(componentsHelmfileUseEKS) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_USE_EKS=%s", componentsHelmfileUseEKS))
		useEKSBool, err := strconv.ParseBool(componentsHelmfileUseEKS)
		if err != nil {
			return err
		}
		cliConfig.Components.Helmfile.UseEKS = useEKSBool
	}

	componentsHelmfileKubeconfigPath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH")
	if len(componentsHelmfileKubeconfigPath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH=%s", componentsHelmfileKubeconfigPath))
		cliConfig.Components.Helmfile.KubeconfigPath = componentsHelmfileKubeconfigPath
	}

	componentsHelmfileHelmAwsProfilePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN")
	if len(componentsHelmfileHelmAwsProfilePattern) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN=%s", componentsHelmfileHelmAwsProfilePattern))
		cliConfig.Components.Helmfile.HelmAwsProfilePattern = componentsHelmfileHelmAwsProfilePattern
	}

	componentsHelmfileClusterNamePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN")
	if len(componentsHelmfileClusterNamePattern) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN=%s", componentsHelmfileClusterNamePattern))
		cliConfig.Components.Helmfile.ClusterNamePattern = componentsHelmfileClusterNamePattern
	}

	workflowsBasePath := os.Getenv("ATMOS_WORKFLOWS_BASE_PATH")
	if len(workflowsBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_WORKFLOWS_BASE_PATH=%s", workflowsBasePath))
		cliConfig.Workflows.BasePath = workflowsBasePath
	}

	jsonschemaBasePath := os.Getenv("ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH")
	if len(jsonschemaBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH=%s", jsonschemaBasePath))
		cliConfig.Schemas.JsonSchema.BasePath = jsonschemaBasePath
	}

	opaBasePath := os.Getenv("ATMOS_SCHEMAS_OPA_BASE_PATH")
	if len(opaBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_OPA_BASE_PATH=%s", opaBasePath))
		cliConfig.Schemas.Opa.BasePath = opaBasePath
	}

	cueBasePath := os.Getenv("ATMOS_SCHEMAS_CUE_BASE_PATH")
	if len(cueBasePath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_CUE_BASE_PATH=%s", cueBasePath))
		cliConfig.Schemas.Cue.BasePath = cueBasePath
	}

	atmosManifestJsonSchemaPath := os.Getenv("ATMOS_SCHEMAS_ATMOS_MANIFEST")
	if len(atmosManifestJsonSchemaPath) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_ATMOS_MANIFEST=%s", atmosManifestJsonSchemaPath))
		cliConfig.Schemas.Atmos.Manifest = atmosManifestJsonSchemaPath
	}

	logsFile := os.Getenv("ATMOS_LOGS_FILE")
	if len(logsFile) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_LOGS_FILE=%s", logsFile))
		cliConfig.Logs.File = logsFile
	}

	logsLevel := os.Getenv("ATMOS_LOGS_LEVEL")
	if len(logsLevel) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_LOGS_LEVEL=%s", logsLevel))
		cliConfig.Logs.Level = logsLevel
	}

	tfAppendUserAgent := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT")
	if len(tfAppendUserAgent) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT=%s", tfAppendUserAgent))
		cliConfig.Components.Terraform.AppendUserAgent = tfAppendUserAgent
	}

	listMergeStrategy := os.Getenv("ATMOS_SETTINGS_LIST_MERGE_STRATEGY")
	if len(listMergeStrategy) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_SETTINGS_LIST_MERGE_STRATEGY=%s", listMergeStrategy))
		cliConfig.Settings.ListMergeStrategy = listMergeStrategy
	}

	versionEnabled := os.Getenv("ATMOS_VERSION_CHECK_ENABLED")
	if len(versionEnabled) > 0 {
		u.LogTrace(*cliConfig, fmt.Sprintf("Found ENV var ATMOS_VERSION_CHECK_ENABLED=%s", versionEnabled))
		enabled, err := strconv.ParseBool(versionEnabled)
		if err != nil {
			u.LogWarning(*cliConfig, fmt.Sprintf("Invalid boolean value '%s' for ATMOS_VERSION_CHECK_ENABLED; using default.", versionEnabled))
		} else {
			cliConfig.Version.Check.Enabled = enabled
		}
	}

	return nil
}

func checkConfig(cliConfig schema.CliConfiguration) error {
	if len(cliConfig.Stacks.BasePath) < 1 {
		return errors.New("stack base path must be provided in 'stacks.base_path' config or ATMOS_STACKS_BASE_PATH' ENV variable")
	}

	if len(cliConfig.Stacks.IncludedPaths) < 1 {
		return errors.New("at least one path must be provided in 'stacks.included_paths' config or ATMOS_STACKS_INCLUDED_PATHS' ENV variable")
	}

	return nil
}

func processCommandLineArgs(cliConfig *schema.CliConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.BasePath) > 0 {
		cliConfig.BasePath = configAndStacksInfo.BasePath
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as base path for stacks and components", configAndStacksInfo.BasePath))
	}
	if len(configAndStacksInfo.TerraformCommand) > 0 {
		cliConfig.Components.Terraform.Command = configAndStacksInfo.TerraformCommand
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as terraform executable", configAndStacksInfo.TerraformCommand))
	}
	if len(configAndStacksInfo.TerraformDir) > 0 {
		cliConfig.Components.Terraform.BasePath = configAndStacksInfo.TerraformDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as terraform directory", configAndStacksInfo.TerraformDir))
	}
	if len(configAndStacksInfo.HelmfileCommand) > 0 {
		cliConfig.Components.Helmfile.Command = configAndStacksInfo.HelmfileCommand
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as helmfile executable", configAndStacksInfo.HelmfileCommand))
	}
	if len(configAndStacksInfo.HelmfileDir) > 0 {
		cliConfig.Components.Helmfile.BasePath = configAndStacksInfo.HelmfileDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as helmfile directory", configAndStacksInfo.HelmfileDir))
	}
	if len(configAndStacksInfo.ConfigDir) > 0 {
		cliConfig.Stacks.BasePath = configAndStacksInfo.ConfigDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as stacks directory", configAndStacksInfo.ConfigDir))
	}
	if len(configAndStacksInfo.StacksDir) > 0 {
		cliConfig.Stacks.BasePath = configAndStacksInfo.StacksDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as stacks directory", configAndStacksInfo.StacksDir))
	}
	if len(configAndStacksInfo.DeployRunInit) > 0 {
		deployRunInitBool, err := strconv.ParseBool(configAndStacksInfo.DeployRunInit)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.DeployRunInit = deployRunInitBool
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s=%s'", DeployRunInitFlag, configAndStacksInfo.DeployRunInit))
	}
	if len(configAndStacksInfo.AutoGenerateBackendFile) > 0 {
		autoGenerateBackendFileBool, err := strconv.ParseBool(configAndStacksInfo.AutoGenerateBackendFile)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.AutoGenerateBackendFile = autoGenerateBackendFileBool
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s=%s'", AutoGenerateBackendFileFlag, configAndStacksInfo.AutoGenerateBackendFile))
	}
	if len(configAndStacksInfo.WorkflowsDir) > 0 {
		cliConfig.Workflows.BasePath = configAndStacksInfo.WorkflowsDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as workflows directory", configAndStacksInfo.WorkflowsDir))
	}
	if len(configAndStacksInfo.InitRunReconfigure) > 0 {
		initRunReconfigureBool, err := strconv.ParseBool(configAndStacksInfo.InitRunReconfigure)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.InitRunReconfigure = initRunReconfigureBool
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s=%s'", InitRunReconfigure, configAndStacksInfo.InitRunReconfigure))
	}
	if len(configAndStacksInfo.JsonSchemaDir) > 0 {
		cliConfig.Schemas.JsonSchema.BasePath = configAndStacksInfo.JsonSchemaDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as JsonSchema schemas directory", configAndStacksInfo.JsonSchemaDir))
	}
	if len(configAndStacksInfo.OpaDir) > 0 {
		cliConfig.Schemas.Opa.BasePath = configAndStacksInfo.OpaDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as OPA schemas directory", configAndStacksInfo.OpaDir))
	}
	if len(configAndStacksInfo.CueDir) > 0 {
		cliConfig.Schemas.Cue.BasePath = configAndStacksInfo.CueDir
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as CUE schemas directory", configAndStacksInfo.CueDir))
	}
	if len(configAndStacksInfo.AtmosManifestJsonSchema) > 0 {
		cliConfig.Schemas.Atmos.Manifest = configAndStacksInfo.AtmosManifestJsonSchema
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s' as path to Atmos JSON Schema", configAndStacksInfo.AtmosManifestJsonSchema))
	}
	if len(configAndStacksInfo.LogsLevel) > 0 {
		cliConfig.Logs.Level = configAndStacksInfo.LogsLevel
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s=%s'", LogsLevelFlag, configAndStacksInfo.LogsLevel))
	}
	if len(configAndStacksInfo.LogsFile) > 0 {
		cliConfig.Logs.File = configAndStacksInfo.LogsFile
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s=%s'", LogsFileFlag, configAndStacksInfo.LogsFile))
	}
	if len(configAndStacksInfo.SettingsListMergeStrategy) > 0 {
		cliConfig.Settings.ListMergeStrategy = configAndStacksInfo.SettingsListMergeStrategy
		u.LogTrace(*cliConfig, fmt.Sprintf("Using command line argument '%s=%s'", SettingsListMergeStrategyFlag, configAndStacksInfo.SettingsListMergeStrategy))
	}

	return nil
}

// GetContextFromVars creates a context object from the provided variables
func GetContextFromVars(vars map[string]any) schema.Context {
	var context schema.Context

	if namespace, ok := vars["namespace"].(string); ok {
		context.Namespace = namespace
	}

	if tenant, ok := vars["tenant"].(string); ok {
		context.Tenant = tenant
	}

	if environment, ok := vars["environment"].(string); ok {
		context.Environment = environment
	}

	if stage, ok := vars["stage"].(string); ok {
		context.Stage = stage
	}

	if region, ok := vars["region"].(string); ok {
		context.Region = region
	}

	if attributes, ok := vars["attributes"].([]any); ok {
		context.Attributes = attributes
	}

	return context
}

// GetContextPrefix calculates context prefix from the context
func GetContextPrefix(stack string, context schema.Context, stackNamePattern string, stackFile string) (string, error) {
	if len(stackNamePattern) == 0 {
		return "",
			errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
	}

	contextPrefix := ""
	stackNamePatternParts := strings.Split(stackNamePattern, "-")

	for _, part := range stackNamePatternParts {
		if part == "{namespace}" {
			if len(context.Namespace) == 0 {
				return "",
					fmt.Errorf("the stack name pattern '%s' specifies 'namespace', but the stack '%s' does not have a namespace defined in the stack file '%s'",
						stackNamePattern,
						stack,
						stackFile,
					)
			}
			if len(contextPrefix) == 0 {
				contextPrefix = context.Namespace
			} else {
				contextPrefix = contextPrefix + "-" + context.Namespace
			}
		} else if part == "{tenant}" {
			if len(context.Tenant) == 0 {
				return "",
					fmt.Errorf("the stack name pattern '%s' specifies 'tenant', but the stack '%s' does not have a tenant defined in the stack file '%s'",
						stackNamePattern,
						stack,
						stackFile,
					)
			}
			if len(contextPrefix) == 0 {
				contextPrefix = context.Tenant
			} else {
				contextPrefix = contextPrefix + "-" + context.Tenant
			}
		} else if part == "{environment}" {
			if len(context.Environment) == 0 {
				return "",
					fmt.Errorf("the stack name pattern '%s' specifies 'environment', but the stack '%s' does not have an environment defined in the stack file '%s'",
						stackNamePattern,
						stack,
						stackFile,
					)
			}
			if len(contextPrefix) == 0 {
				contextPrefix = context.Environment
			} else {
				contextPrefix = contextPrefix + "-" + context.Environment
			}
		} else if part == "{stage}" {
			if len(context.Stage) == 0 {
				return "",
					fmt.Errorf("the stack name pattern '%s' specifies 'stage', but the stack '%s' does not have a stage defined in the stack file '%s'",
						stackNamePattern,
						stack,
						stackFile,
					)
			}
			if len(contextPrefix) == 0 {
				contextPrefix = context.Stage
			} else {
				contextPrefix = contextPrefix + "-" + context.Stage
			}
		}
	}

	return contextPrefix, nil
}

// ReplaceContextTokens replaces context tokens in the provided pattern and returns a string with all the tokens replaced
func ReplaceContextTokens(context schema.Context, pattern string) string {
	r := strings.NewReplacer(
		"{base-component}", context.BaseComponent,
		"{component}", context.Component,
		"{component-path}", context.ComponentPath,
		"{namespace}", context.Namespace,
		"{environment}", context.Environment,
		"{region}", context.Region,
		"{tenant}", context.Tenant,
		"{stage}", context.Stage,
		"{workspace}", context.Workspace,
		"{terraform_workspace}", context.TerraformWorkspace,
		"{attributes}", strings.Join(u.SliceOfInterfacesToSliceOdStrings(context.Attributes), "-"),
	)
	return r.Replace(pattern)
}

// GetStackNameFromContextAndStackNamePattern calculates stack name from the provided context using the provided stack name pattern
func GetStackNameFromContextAndStackNamePattern(
	namespace string,
	tenant string,
	environment string,
	stage string,
	stackNamePattern string,
) (string, error) {

	if len(stackNamePattern) == 0 {
		return "",
			fmt.Errorf("stack name pattern must be provided")
	}

	var stack string
	stackNamePatternParts := strings.Split(stackNamePattern, "-")

	for _, part := range stackNamePatternParts {
		if part == "{namespace}" {
			if len(namespace) == 0 {
				return "", fmt.Errorf("stack name pattern '%s' includes '{namespace}', but namespace is not provided", stackNamePattern)
			}
			if len(stack) == 0 {
				stack = namespace
			} else {
				stack = fmt.Sprintf("%s-%s", stack, namespace)
			}
		} else if part == "{tenant}" {
			if len(tenant) == 0 {
				return "", fmt.Errorf("stack name pattern '%s' includes '{tenant}', but tenant is not provided", stackNamePattern)
			}
			if len(stack) == 0 {
				stack = tenant
			} else {
				stack = fmt.Sprintf("%s-%s", stack, tenant)
			}
		} else if part == "{environment}" {
			if len(environment) == 0 {
				return "", fmt.Errorf("stack name pattern '%s' includes '{environment}', but environment is not provided", stackNamePattern)
			}
			if len(stack) == 0 {
				stack = environment
			} else {
				stack = fmt.Sprintf("%s-%s", stack, environment)
			}
		} else if part == "{stage}" {
			if len(stage) == 0 {
				return "", fmt.Errorf("stack name pattern '%s' includes '{stage}', but stage is not provided", stackNamePattern)
			}
			if len(stack) == 0 {
				stack = stage
			} else {
				stack = fmt.Sprintf("%s-%s", stack, stage)
			}
		}
	}

	return stack, nil
}

// getStackFilePatterns returns a slice of possible file patterns for a given base path
func getStackFilePatterns(basePath string, includeTemplates bool) []string {
	patterns := []string{
		basePath + u.YamlFileExtension,
		basePath + u.YmlFileExtension,
	}

	if includeTemplates {
		patterns = append(patterns,
			basePath+u.YamlTemplateExtension,
			basePath+u.YmlTemplateExtension,
		)
	}

	return patterns
}

// matchesStackFilePattern checks if a file path matches any of the valid stack file patterns
func matchesStackFilePattern(filePath, stackName string) bool {
	// Always include template files for normal operations (imports, etc.)
	patterns := getStackFilePatterns(stackName, true)
	for _, pattern := range patterns {
		if strings.HasSuffix(filePath, pattern) {
			return true
		}
	}
	return false
}

func getConfigFilePatterns(path string, forGlobMatch bool) []string {
	if path == "" {
		return []string{}
	}
	ext := filepath.Ext(path)
	if ext != "" {
		return []string{path}
	}

	// include template files for normal operations
	patterns := getStackFilePatterns(path, true)
	if !forGlobMatch {
		// For direct file search, include the exact path without extension
		patterns = append([]string{path}, patterns...)
	}

	return patterns
}

func SearchConfigFile(configPath string, cliConfig schema.CliConfiguration) (string, error) {
	// If path already has an extension, verify it exists
	if ext := filepath.Ext(configPath); ext != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
		return "", fmt.Errorf("specified config file not found: %s", configPath)
	}

	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("error reading directory %s: %v", dir, err)
	}

	// Create a map of existing files for quick lookup
	fileMap := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			fileMap[entry.Name()] = true
		}
	}

	// Try all patterns in order
	patterns := getConfigFilePatterns(base, false)
	for _, pattern := range patterns {
		if fileMap[pattern] {
			return filepath.Join(dir, pattern), nil
		}
	}

	return "", fmt.Errorf("failed to find a match for the import '%s' ('%s' + '%s')", configPath, dir, base)
}
