package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/cloudposse/atmos/pkg/globals"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// FindAllStackConfigsInPathsForStack finds all stack config files in the paths specified by globs for the provided stack
func FindAllStackConfigsInPathsForStack(
	cliConfig CliConfiguration,
	stack string,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, bool, error) {

	var absolutePaths []string
	var relativePaths []string
	var stackIsDir = strings.IndexAny(stack, "/") > 0

	for _, p := range includeStackPaths {
		pathWithExt := p

		ext := filepath.Ext(p)
		if ext == "" {
			ext = g.DefaultStackConfigFileExtension
			pathWithExt = p + ext
		}

		// Find all matches in the glob
		matches, err := u.GetGlobMatches(pathWithExt)
		if err != nil || len(matches) == 0 {
			// Retry (b/c we are using `doublestar` library, and it sometimes has issues reading many files in a Docker container)
			// TODO: review `doublestar` library
			matches, err = u.GetGlobMatches(pathWithExt)
			if err != nil {
				return nil, nil, false, err
			}
		}

		// Exclude files that match any of the excludePaths
		for _, matchedFileAbsolutePath := range matches {
			matchedFileRelativePath := u.TrimBasePathFromPath(cliConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)

			// Check if the provided stack matches a file in the config folders (excluding the files from `excludeStackPaths`)
			stackMatch := strings.HasSuffix(matchedFileAbsolutePath, stack+g.DefaultStackConfigFileExtension)

			if stackMatch {
				allExcluded := true
				for _, excludePath := range excludeStackPaths {
					excludeMatch, err := u.PathMatch(excludePath, matchedFileAbsolutePath)
					if err != nil {
						u.PrintError(err)
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
					u.PrintError(err)
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

	return absolutePaths, relativePaths, false, nil
}

// FindAllStackConfigsInPaths finds all stack config files in the paths specified by globs
func FindAllStackConfigsInPaths(
	cliConfig CliConfiguration,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, error) {

	var absolutePaths []string
	var relativePaths []string

	for _, p := range includeStackPaths {
		pathWithExt := p

		ext := filepath.Ext(p)
		if ext == "" {
			ext = g.DefaultStackConfigFileExtension
			pathWithExt = p + ext
		}

		// Find all matches in the glob
		matches, err := u.GetGlobMatches(pathWithExt)
		if err != nil || len(matches) == 0 {
			// Retry (b/c we are using `doublestar` library, and it sometimes has issues reading many files in a Docker container)
			// TODO: review `doublestar` library
			matches, err = u.GetGlobMatches(pathWithExt)
			if err != nil {
				return nil, nil, err
			}
		}

		// Exclude files that match any of the excludePaths
		for _, matchedFileAbsolutePath := range matches {
			matchedFileRelativePath := u.TrimBasePathFromPath(cliConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)
			include := true

			for _, excludePath := range excludeStackPaths {
				excludeMatch, err := u.PathMatch(excludePath, matchedFileAbsolutePath)
				if err != nil {
					u.PrintError(err)
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

func processEnvVars(cliConfig CliConfiguration) error {
	basePath := os.Getenv("ATMOS_BASE_PATH")
	if len(basePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_BASE_PATH=%s", basePath))
		cliConfig.BasePath = basePath
	}

	stacksBasePath := os.Getenv("ATMOS_STACKS_BASE_PATH")
	if len(stacksBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_STACKS_BASE_PATH=%s", stacksBasePath))
		cliConfig.Stacks.BasePath = stacksBasePath
	}

	stacksIncludedPaths := os.Getenv("ATMOS_STACKS_INCLUDED_PATHS")
	if len(stacksIncludedPaths) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_STACKS_INCLUDED_PATHS=%s", stacksIncludedPaths))
		cliConfig.Stacks.IncludedPaths = strings.Split(stacksIncludedPaths, ",")
	}

	stacksExcludedPaths := os.Getenv("ATMOS_STACKS_EXCLUDED_PATHS")
	if len(stacksExcludedPaths) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_STACKS_EXCLUDED_PATHS=%s", stacksExcludedPaths))
		cliConfig.Stacks.ExcludedPaths = strings.Split(stacksExcludedPaths, ",")
	}

	stacksNamePattern := os.Getenv("ATMOS_STACKS_NAME_PATTERN")
	if len(stacksNamePattern) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_STACKS_NAME_PATTERN=%s", stacksNamePattern))
		cliConfig.Stacks.NamePattern = stacksNamePattern
	}

	componentsTerraformBasePath := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH")
	if len(componentsTerraformBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_BASE_PATH=%s", componentsTerraformBasePath))
		cliConfig.Components.Terraform.BasePath = componentsTerraformBasePath
	}

	componentsTerraformApplyAutoApprove := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE")
	if len(componentsTerraformApplyAutoApprove) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=%s", componentsTerraformApplyAutoApprove))
		applyAutoApproveBool, err := strconv.ParseBool(componentsTerraformApplyAutoApprove)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.ApplyAutoApprove = applyAutoApproveBool
	}

	componentsTerraformDeployRunInit := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT")
	if len(componentsTerraformDeployRunInit) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT=%s", componentsTerraformDeployRunInit))
		deployRunInitBool, err := strconv.ParseBool(componentsTerraformDeployRunInit)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.DeployRunInit = deployRunInitBool
	}

	componentsInitRunReconfigure := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE")
	if len(componentsInitRunReconfigure) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE=%s", componentsInitRunReconfigure))
		initRunReconfigureBool, err := strconv.ParseBool(componentsInitRunReconfigure)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.InitRunReconfigure = initRunReconfigureBool
	}

	componentsTerraformAutoGenerateBackendFile := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE")
	if len(componentsTerraformAutoGenerateBackendFile) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE=%s", componentsTerraformAutoGenerateBackendFile))
		componentsTerraformAutoGenerateBackendFileBool, err := strconv.ParseBool(componentsTerraformAutoGenerateBackendFile)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.AutoGenerateBackendFile = componentsTerraformAutoGenerateBackendFileBool
	}

	componentsHelmfileBasePath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH")
	if len(componentsHelmfileBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_BASE_PATH=%s", componentsHelmfileBasePath))
		cliConfig.Components.Helmfile.BasePath = componentsHelmfileBasePath
	}

	componentsHelmfileKubeconfigPath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH")
	if len(componentsHelmfileKubeconfigPath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH=%s", componentsHelmfileKubeconfigPath))
		cliConfig.Components.Helmfile.KubeconfigPath = componentsHelmfileKubeconfigPath
	}

	componentsHelmfileHelmAwsProfilePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN")
	if len(componentsHelmfileHelmAwsProfilePattern) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN=%s", componentsHelmfileHelmAwsProfilePattern))
		cliConfig.Components.Helmfile.HelmAwsProfilePattern = componentsHelmfileHelmAwsProfilePattern
	}

	componentsHelmfileClusterNamePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN")
	if len(componentsHelmfileClusterNamePattern) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN=%s", componentsHelmfileClusterNamePattern))
		cliConfig.Components.Helmfile.ClusterNamePattern = componentsHelmfileClusterNamePattern
	}

	workflowsBasePath := os.Getenv("ATMOS_WORKFLOWS_BASE_PATH")
	if len(workflowsBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_WORKFLOWS_BASE_PATH=%s", workflowsBasePath))
		cliConfig.Workflows.BasePath = workflowsBasePath
	}

	jsonschemaBasePath := os.Getenv("ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH")
	if len(jsonschemaBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH=%s", jsonschemaBasePath))
		cliConfig.Schemas.JsonSchema.BasePath = jsonschemaBasePath
	}

	opaBasePath := os.Getenv("ATMOS_SCHEMAS_OPA_BASE_PATH")
	if len(opaBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_OPA_BASE_PATH=%s", opaBasePath))
		cliConfig.Schemas.Opa.BasePath = opaBasePath
	}

	cueBasePath := os.Getenv("ATMOS_SCHEMAS_CUE_BASE_PATH")
	if len(cueBasePath) > 0 {
		u.PrintInfoVerbose(fmt.Sprintf("Found ENV var ATMOS_SCHEMAS_CUE_BASE_PATH=%s", cueBasePath))
		cliConfig.Schemas.Cue.BasePath = cueBasePath
	}

	return nil
}

func checkConfig(cliConfig CliConfiguration) error {
	if len(cliConfig.Stacks.BasePath) < 1 {
		return errors.New("stack base path must be provided in 'stacks.base_path' config or ATMOS_STACKS_BASE_PATH' ENV variable")
	}

	if len(cliConfig.Stacks.IncludedPaths) < 1 {
		return errors.New("at least one path must be provided in 'stacks.included_paths' config or ATMOS_STACKS_INCLUDED_PATHS' ENV variable")
	}

	return nil
}

func processCommandLineArgs(cliConfig CliConfiguration, configAndStacksInfo ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.BasePath) > 0 {
		cliConfig.BasePath = configAndStacksInfo.BasePath
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as base path for stacks and components", configAndStacksInfo.BasePath))
	}
	if len(configAndStacksInfo.TerraformDir) > 0 {
		cliConfig.Components.Terraform.BasePath = configAndStacksInfo.TerraformDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as terraform directory", configAndStacksInfo.TerraformDir))
	}
	if len(configAndStacksInfo.HelmfileDir) > 0 {
		cliConfig.Components.Helmfile.BasePath = configAndStacksInfo.HelmfileDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as helmfile directory", configAndStacksInfo.HelmfileDir))
	}
	if len(configAndStacksInfo.ConfigDir) > 0 {
		cliConfig.Stacks.BasePath = configAndStacksInfo.ConfigDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as stacks directory", configAndStacksInfo.ConfigDir))
	}
	if len(configAndStacksInfo.StacksDir) > 0 {
		cliConfig.Stacks.BasePath = configAndStacksInfo.StacksDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as stacks directory", configAndStacksInfo.StacksDir))
	}
	if len(configAndStacksInfo.DeployRunInit) > 0 {
		deployRunInitBool, err := strconv.ParseBool(configAndStacksInfo.DeployRunInit)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.DeployRunInit = deployRunInitBool
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s=%s'", g.DeployRunInitFlag, configAndStacksInfo.DeployRunInit))
	}
	if len(configAndStacksInfo.AutoGenerateBackendFile) > 0 {
		autoGenerateBackendFileBool, err := strconv.ParseBool(configAndStacksInfo.AutoGenerateBackendFile)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.AutoGenerateBackendFile = autoGenerateBackendFileBool
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s=%s'", g.AutoGenerateBackendFileFlag, configAndStacksInfo.AutoGenerateBackendFile))
	}
	if len(configAndStacksInfo.WorkflowsDir) > 0 {
		cliConfig.Workflows.BasePath = configAndStacksInfo.WorkflowsDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as workflows directory", configAndStacksInfo.WorkflowsDir))
	}
	if len(configAndStacksInfo.InitRunReconfigure) > 0 {
		initRunReconfigureBool, err := strconv.ParseBool(configAndStacksInfo.InitRunReconfigure)
		if err != nil {
			return err
		}
		cliConfig.Components.Terraform.InitRunReconfigure = initRunReconfigureBool
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s=%s'", g.InitRunReconfigure, configAndStacksInfo.InitRunReconfigure))
	}
	if len(configAndStacksInfo.JsonSchemaDir) > 0 {
		cliConfig.Schemas.JsonSchema.BasePath = configAndStacksInfo.JsonSchemaDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as JsonSchema schemas directory", configAndStacksInfo.JsonSchemaDir))
	}
	if len(configAndStacksInfo.OpaDir) > 0 {
		cliConfig.Schemas.Opa.BasePath = configAndStacksInfo.OpaDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as OPA schemas directory", configAndStacksInfo.OpaDir))
	}
	if len(configAndStacksInfo.CueDir) > 0 {
		cliConfig.Schemas.Cue.BasePath = configAndStacksInfo.CueDir
		u.PrintInfoVerbose(fmt.Sprintf("Using command line argument '%s' as CUE schemas directory", configAndStacksInfo.CueDir))
	}

	return nil
}

func processLogsConfig(cliConfig CliConfiguration) error {
	logVerbose := os.Getenv("ATMOS_LOGS_VERBOSE")
	if len(logVerbose) > 0 {
		u.PrintInfo(fmt.Sprintf("Found ENV var ATMOS_LOGS_VERBOSE=%s", logVerbose))
		logVerboseBool, err := strconv.ParseBool(logVerbose)
		if err != nil {
			return err
		}
		cliConfig.Logs.Verbose = logVerboseBool
		g.LogVerbose = logVerboseBool
	}
	return nil
}

// GetContextFromVars creates a context object from the provided variables
func GetContextFromVars(vars map[any]any) Context {
	var context Context

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

	if attributes, ok := vars["attributes"].([]string); ok {
		context.Attributes = attributes
	}

	return context
}

// GetContextPrefix calculates context prefix from the context
func GetContextPrefix(stack string, context Context, stackNamePattern string, stackFile string) (string, error) {
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
					fmt.Errorf("the stack name pattern '%s' specifies 'namespace`, but the stack '%s' does not have a namespace defined in the stack file '%s'",
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
					fmt.Errorf("the stack name pattern '%s' specifies 'tenant`, but the stack '%s' does not have a tenant defined in the stack file '%s'",
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
					fmt.Errorf("the stack name pattern '%s' specifies 'environment`, but the stack '%s' does not have an environment defined in the stack file '%s'",
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
					fmt.Errorf("the stack name pattern '%s' specifies 'stage`, but the stack '%s' does not have a stage defined in the stack file '%s'",
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
func ReplaceContextTokens(context Context, pattern string) string {
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
		"{attributes}", strings.Join(context.Attributes, "-"),
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
