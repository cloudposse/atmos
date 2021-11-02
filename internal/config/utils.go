package config

import (
	"errors"
	"github.com/bmatcuk/doublestar"
	g "github.com/cloudposse/atmos/internal/globals"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	// Mutex to serialize reading YAML config files
	readYAMLConfigFileLock = &sync.Mutex{}
)

// findAllStackConfigsInPaths finds all stack config files in the paths specified by globs
func findAllStackConfigsInPaths(
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
		readYAMLConfigFileLock.Lock()
		matches, err := doublestar.Glob(pathWithExt)
		readYAMLConfigFileLock.Unlock()
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
