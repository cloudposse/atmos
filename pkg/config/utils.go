package config

import (
	"errors"
	"fmt"
	"github.com/bmatcuk/doublestar/v4"
	g "github.com/cloudposse/atmos/pkg/globals"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// findAllStackConfigsInPathsForStack finds all stack config files in the paths specified by globs for the provided stack
func findAllStackConfigsInPathsForStack(
	stack string,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, bool, error) {

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
		matches, err := s.GetGlobMatches(pathWithExt)
		if err != nil {
			return nil, nil, false, err
		}

		// Exclude files that match any of the excludePaths
		if matches != nil && len(matches) > 0 {
			for _, matchedFileAbsolutePath := range matches {
				matchedFileRelativePath := u.TrimBasePathFromPath(ProcessedConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)

				// Check if the provided stack matches a file in the config folders (excluding the files from `excludeStackPaths`)
				stackMatch := strings.HasSuffix(matchedFileAbsolutePath, stack+g.DefaultStackConfigFileExtension)
				if stackMatch == true {
					allExcluded := true
					for _, excludePath := range excludeStackPaths {
						excludeMatch, err := doublestar.PathMatch(excludePath, matchedFileAbsolutePath)
						if err != nil {
							color.Red("%s", err)
							continue
						} else if excludeMatch == true {
							allExcluded = false
							break
						}
					}
					if allExcluded == true {
						return []string{matchedFileAbsolutePath}, []string{matchedFileRelativePath}, true, nil
					}
				}

				include := true

				for _, excludePath := range excludeStackPaths {
					excludeMatch, err := doublestar.PathMatch(excludePath, matchedFileAbsolutePath)
					if err != nil {
						color.Red("%s", err)
						include = false
						continue
					} else if excludeMatch {
						include = false
						continue
					}
				}

				if include == true {
					absolutePaths = append(absolutePaths, matchedFileAbsolutePath)
					relativePaths = append(relativePaths, matchedFileRelativePath)
				}
			}
		}
	}

	return absolutePaths, relativePaths, false, nil
}

func processEnvVars() error {
	stacksBasePath := os.Getenv("ATMOS_STACKS_BASE_PATH")
	if len(stacksBasePath) > 0 {
		color.Cyan("Found ENV var ATMOS_STACKS_BASE_PATH=%s", stacksBasePath)
		Config.Stacks.BasePath = stacksBasePath
	}

	stacksIncludedPaths := os.Getenv("ATMOS_STACKS_INCLUDED_PATHS")
	if len(stacksIncludedPaths) > 0 {
		color.Cyan("Found ENV var ATMOS_STACKS_INCLUDED_PATHS=%s", stacksIncludedPaths)
		Config.Stacks.IncludedPaths = strings.Split(stacksIncludedPaths, ",")
	}

	stacksExcludedPaths := os.Getenv("ATMOS_STACKS_EXCLUDED_PATHS")
	if len(stacksExcludedPaths) > 0 {
		color.Cyan("Found ENV var ATMOS_STACKS_EXCLUDED_PATHS=%s", stacksExcludedPaths)
		Config.Stacks.ExcludedPaths = strings.Split(stacksExcludedPaths, ",")
	}

	stacksNamePattern := os.Getenv("ATMOS_STACKS_NAME_PATTERN")
	if len(stacksNamePattern) > 0 {
		color.Cyan("Found ENV var ATMOS_STACKS_NAME_PATTERN=%s", stacksNamePattern)
		Config.Stacks.NamePattern = stacksNamePattern
	}

	componentsTerraformBasePath := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH")
	if len(componentsTerraformBasePath) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_TERRAFORM_BASE_PATH=%s", componentsTerraformBasePath)
		Config.Components.Terraform.BasePath = componentsTerraformBasePath
	}

	componentsTerraformApplyAutoApprove := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE")
	if len(componentsTerraformApplyAutoApprove) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=%s", componentsTerraformApplyAutoApprove)
		applyAutoApproveBool, err := strconv.ParseBool(componentsTerraformApplyAutoApprove)
		if err != nil {
			return err
		}
		Config.Components.Terraform.ApplyAutoApprove = applyAutoApproveBool
	}

	componentsTerraformDeployRunInit := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT")
	if len(componentsTerraformDeployRunInit) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT=%s", componentsTerraformDeployRunInit)
		deployRunInitBool, err := strconv.ParseBool(componentsTerraformDeployRunInit)
		if err != nil {
			return err
		}
		Config.Components.Terraform.DeployRunInit = deployRunInitBool
	}

	componentsTerraformAutoGenerateBackendFile := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE")
	if len(componentsTerraformAutoGenerateBackendFile) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE=%s", componentsTerraformAutoGenerateBackendFile)
		componentsTerraformAutoGenerateBackendFileBool, err := strconv.ParseBool(componentsTerraformAutoGenerateBackendFile)
		if err != nil {
			return err
		}
		Config.Components.Terraform.AutoGenerateBackendFile = componentsTerraformAutoGenerateBackendFileBool
	}

	componentsHelmfileBasePath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH")
	if len(componentsHelmfileBasePath) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_HELMFILE_BASE_PATH=%s", componentsHelmfileBasePath)
		Config.Components.Helmfile.BasePath = componentsHelmfileBasePath
	}

	componentsHelmfileKubeconfigPath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH")
	if len(componentsHelmfileKubeconfigPath) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH=%s", componentsHelmfileKubeconfigPath)
		Config.Components.Helmfile.KubeconfigPath = componentsHelmfileKubeconfigPath
	}

	componentsHelmfileHelmAwsProfilePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN")
	if len(componentsHelmfileHelmAwsProfilePattern) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN=%s", componentsHelmfileHelmAwsProfilePattern)
		Config.Components.Helmfile.HelmAwsProfilePattern = componentsHelmfileHelmAwsProfilePattern
	}

	componentsHelmfileClusterNamePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN")
	if len(componentsHelmfileClusterNamePattern) > 0 {
		color.Cyan("Found ENV var ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN=%s", componentsHelmfileClusterNamePattern)
		Config.Components.Helmfile.ClusterNamePattern = componentsHelmfileClusterNamePattern
	}

	return nil
}

func checkConfig() error {
	if len(Config.Stacks.BasePath) < 1 {
		return errors.New("stack base path must be provided in 'stacks.base_path' config or ATMOS_STACKS_BASE_PATH' ENV variable")
	}

	if len(Config.Stacks.IncludedPaths) < 1 {
		return errors.New("at least one path must be provided in 'stacks.included_paths' config or ATMOS_STACKS_INCLUDED_PATHS' ENV variable")
	}

	return nil
}

func processLogsConfig() error {
	logVerbose := os.Getenv("ATMOS_LOGS_VERBOSE")
	if len(logVerbose) > 0 {
		color.Cyan("Found ENV var ATMOS_LOGS_VERBOSE=%s", logVerbose)
		logVerboseBool, err := strconv.ParseBool(logVerbose)
		if err != nil {
			return err
		}
		Config.Logs.Verbose = logVerboseBool
		g.LogVerbose = logVerboseBool
	}
	return nil
}

// GetContextFromVars creates a context object from the provided variables
func GetContextFromVars(vars map[interface{}]interface{}) Context {
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

	return context
}

// GetContextPrefix calculates context prefix
func GetContextPrefix(stack string, context Context, stackNamePattern string) (string, error) {
	if len(stackNamePattern) == 0 {
		return "",
			errors.New(fmt.Sprintf("Stack name pattern must be provided"))
	}

	contextPrefix := ""
	stackNamePatternParts := strings.Split(stackNamePattern, "-")

	for _, part := range stackNamePatternParts {
		if part == "{tenant}" {
			if len(context.Tenant) == 0 {
				return "",
					errors.New(fmt.Sprintf("The stack name pattern '%s' specifies 'tenant`, but the stack %s does not have a tenant defined",
						stackNamePattern,
						stack,
					))
			}
			if len(contextPrefix) == 0 {
				contextPrefix = context.Tenant
			} else {
				contextPrefix = contextPrefix + "-" + context.Tenant
			}
		} else if part == "{environment}" {
			if len(context.Environment) == 0 {
				return "",
					errors.New(fmt.Sprintf("The stack name pattern '%s' specifies 'environment`, but the stack %s does not have an environment defined",
						stackNamePattern,
						stack,
					))
			}
			if len(contextPrefix) == 0 {
				contextPrefix = context.Environment
			} else {
				contextPrefix = contextPrefix + "-" + context.Environment
			}
		} else if part == "{stage}" {
			if len(context.Stage) == 0 {
				return "",
					errors.New(fmt.Sprintf("The stack name pattern '%s' specifies 'stage`, but the stack %s does not have a stage defined",
						Config.Stacks.NamePattern,
						stack,
					))
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

// ReplaceContextTokens replaces tokens in the context pattern
func ReplaceContextTokens(context Context, pattern string) string {
	return strings.Replace(
		strings.Replace(
			strings.Replace(
				strings.Replace(pattern,
					"{namespace}", context.Namespace, 1),
				"{environment}", context.Environment, 1),
			"{tenant}", context.Tenant, 1),
		"{stage}", context.Stage, 1)
}
