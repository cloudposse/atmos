package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

// FindAllStackConfigsInPathsForStack finds all stack manifests in the paths specified by globs for the provided stack.
func FindAllStackConfigsInPathsForStack(
	atmosConfig schema.AtmosConfiguration,
	stack string,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, bool, error) {
	var absolutePaths []string
	var relativePaths []string
	stackIsDir := strings.IndexAny(stack, "/") > 0

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
				return nil, nil, false, err
			}
			// If there's no error but still no matches, we continue to the next path
			// This happens when the pattern is valid but no files match it
			continue
		}

		// Process all matches found
		for _, matchedFileAbsolutePath := range allMatches {
			matchedFileRelativePath := u.TrimBasePathFromPath(atmosConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)

			// Check if the provided stack matches a file in the config folders (excluding the files from `excludeStackPaths`)
			stackMatch := matchesStackFilePattern(filepath.ToSlash(matchedFileAbsolutePath), stack)

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

// FindAllStackConfigsInPaths finds all stack manifests in the paths specified by globs.
func FindAllStackConfigsInPaths(
	atmosConfig *schema.AtmosConfiguration,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, error) {
	defer perf.Track(atmosConfig, "config.FindAllStackConfigsInPaths")()

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
				return nil, nil, err
			}
			// If there's no error but still no matches, we continue to the next path
			// This happens when the pattern is valid but no files match it
			continue
		}

		// Process all matches found
		for _, matchedFileAbsolutePath := range allMatches {
			matchedFileRelativePath := u.TrimBasePathFromPath(atmosConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)
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

func processEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "config.processEnvVars")()

	foundEnvVarMessage := "Found ENV variable"

	basePath := os.Getenv("ATMOS_BASE_PATH")
	if len(basePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_BASE_PATH", basePath)
		atmosConfig.BasePath = basePath
	}

	vendorBasePath := os.Getenv("ATMOS_VENDOR_BASE_PATH")
	if len(vendorBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_VENDOR_BASE_PATH", vendorBasePath)
		atmosConfig.Vendor.BasePath = vendorBasePath
	}

	stacksBasePath := os.Getenv("ATMOS_STACKS_BASE_PATH")
	if len(stacksBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_BASE_PATH", stacksBasePath)
		atmosConfig.Stacks.BasePath = stacksBasePath
	}

	stacksIncludedPaths := os.Getenv("ATMOS_STACKS_INCLUDED_PATHS")
	if len(stacksIncludedPaths) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_INCLUDED_PATHS", stacksIncludedPaths)
		atmosConfig.Stacks.IncludedPaths = strings.Split(stacksIncludedPaths, ",")
	}

	stacksExcludedPaths := os.Getenv("ATMOS_STACKS_EXCLUDED_PATHS")
	if len(stacksExcludedPaths) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_EXCLUDED_PATHS", stacksExcludedPaths)
		atmosConfig.Stacks.ExcludedPaths = strings.Split(stacksExcludedPaths, ",")
	}

	stacksNamePattern := os.Getenv("ATMOS_STACKS_NAME_PATTERN")
	if len(stacksNamePattern) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_NAME_PATTERN", stacksNamePattern)
		atmosConfig.Stacks.NamePattern = stacksNamePattern
	}

	stacksNameTemplate := os.Getenv("ATMOS_STACKS_NAME_TEMPLATE")
	if len(stacksNameTemplate) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_NAME_TEMPLATE", stacksNameTemplate)
		atmosConfig.Stacks.NameTemplate = stacksNameTemplate
	}

	componentsTerraformCommand := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND")
	if len(componentsTerraformCommand) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_COMMAND", componentsTerraformCommand)
		atmosConfig.Components.Terraform.Command = componentsTerraformCommand
	}

	componentsTerraformBasePath := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH")
	if len(componentsTerraformBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", componentsTerraformBasePath)
		atmosConfig.Components.Terraform.BasePath = componentsTerraformBasePath
	}

	componentsTerraformApplyAutoApprove := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE")
	if len(componentsTerraformApplyAutoApprove) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE", componentsTerraformApplyAutoApprove)
		applyAutoApproveBool, err := strconv.ParseBool(componentsTerraformApplyAutoApprove)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.ApplyAutoApprove = applyAutoApproveBool
	}

	componentsTerraformDeployRunInit := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT")
	if len(componentsTerraformDeployRunInit) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT", componentsTerraformDeployRunInit)
		deployRunInitBool, err := strconv.ParseBool(componentsTerraformDeployRunInit)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.DeployRunInit = deployRunInitBool
	}

	componentsInitRunReconfigure := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE")
	if len(componentsInitRunReconfigure) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE", componentsInitRunReconfigure)
		initRunReconfigureBool, err := strconv.ParseBool(componentsInitRunReconfigure)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.InitRunReconfigure = initRunReconfigureBool
	}

	componentsInitPassVars := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_INIT_PASS_VARS")
	if len(componentsInitPassVars) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_INIT_PASS_VARS", componentsInitPassVars)
		initPassVarsBool, err := strconv.ParseBool(componentsInitPassVars)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.Init.PassVars = initPassVarsBool
	}

	componentsPlanSkipPlanfile := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_PLAN_SKIP_PLANFILE")
	if len(componentsPlanSkipPlanfile) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_PLAN_SKIP_PLANFILE", componentsPlanSkipPlanfile)
		planSkipPlanfileBool, err := strconv.ParseBool(componentsPlanSkipPlanfile)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.Plan.SkipPlanfile = planSkipPlanfileBool
	}

	componentsTerraformAutoGenerateBackendFile := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE")
	if len(componentsTerraformAutoGenerateBackendFile) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE", componentsTerraformAutoGenerateBackendFile)
		componentsTerraformAutoGenerateBackendFileBool, err := strconv.ParseBool(componentsTerraformAutoGenerateBackendFile)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.AutoGenerateBackendFile = componentsTerraformAutoGenerateBackendFileBool
	}

	componentsHelmfileCommand := os.Getenv("ATMOS_COMPONENTS_HELMFILE_COMMAND")
	if len(componentsHelmfileCommand) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_COMMAND", componentsHelmfileCommand)
		atmosConfig.Components.Helmfile.Command = componentsHelmfileCommand
	}

	componentsHelmfileBasePath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH")
	if len(componentsHelmfileBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_BASE_PATH", componentsHelmfileBasePath)
		atmosConfig.Components.Helmfile.BasePath = componentsHelmfileBasePath
	}

	componentsHelmfileUseEKS := os.Getenv("ATMOS_COMPONENTS_HELMFILE_USE_EKS")
	if len(componentsHelmfileUseEKS) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_USE_EKS", componentsHelmfileUseEKS)
		useEKSBool, err := strconv.ParseBool(componentsHelmfileUseEKS)
		if err != nil {
			return err
		}
		atmosConfig.Components.Helmfile.UseEKS = useEKSBool
	}

	componentsHelmfileKubeconfigPath := os.Getenv("ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH")
	if len(componentsHelmfileKubeconfigPath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH", componentsHelmfileKubeconfigPath)
		atmosConfig.Components.Helmfile.KubeconfigPath = componentsHelmfileKubeconfigPath
	}

	componentsHelmfileHelmAwsProfilePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN")
	if len(componentsHelmfileHelmAwsProfilePattern) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN", componentsHelmfileHelmAwsProfilePattern)
		atmosConfig.Components.Helmfile.HelmAwsProfilePattern = componentsHelmfileHelmAwsProfilePattern
	}

	componentsHelmfileClusterNamePattern := os.Getenv("ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN")
	if len(componentsHelmfileClusterNamePattern) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN", componentsHelmfileClusterNamePattern)
		atmosConfig.Components.Helmfile.ClusterNamePattern = componentsHelmfileClusterNamePattern
	}

	componentsPackerCommand := os.Getenv("ATMOS_COMPONENTS_PACKER_COMMAND")
	if len(componentsPackerCommand) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_PACKER_COMMAND", componentsPackerCommand)
		atmosConfig.Components.Packer.Command = componentsPackerCommand
	}

	componentsPackerBasePath := os.Getenv("ATMOS_COMPONENTS_PACKER_BASE_PATH")
	if len(componentsPackerBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_PACKER_BASE_PATH", componentsPackerBasePath)
		atmosConfig.Components.Packer.BasePath = componentsPackerBasePath
	}

	workflowsBasePath := os.Getenv("ATMOS_WORKFLOWS_BASE_PATH")
	if len(workflowsBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_WORKFLOWS_BASE_PATH", workflowsBasePath)
		atmosConfig.Workflows.BasePath = workflowsBasePath
	}

	jsonschemaBasePath := os.Getenv("ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH")
	if len(jsonschemaBasePath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"jsonschema\"] using ENV variable", "ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH", jsonschemaBasePath)
		atmosConfig.Schemas["jsonschema"] = schema.ResourcePath{
			BasePath: jsonschemaBasePath,
		}
	}

	opaBasePath := os.Getenv("ATMOS_SCHEMAS_OPA_BASE_PATH")
	if len(opaBasePath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"opa\"] using ENV variable", "ATMOS_SCHEMAS_OPA_BASE_PATH", opaBasePath)
		atmosConfig.Schemas["opa"] = schema.ResourcePath{
			BasePath: opaBasePath,
		}
	}

	cueBasePath := os.Getenv("ATMOS_SCHEMAS_CUE_BASE_PATH")
	if len(cueBasePath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"cue\"] using ENV variable", "ATMOS_SCHEMAS_CUE_BASE_PATH", cueBasePath)
		atmosConfig.Schemas["cue"] = schema.ResourcePath{
			BasePath: cueBasePath,
		}
	}

	atmosManifestJsonSchemaPath := os.Getenv("ATMOS_SCHEMAS_ATMOS_MANIFEST")
	if len(atmosManifestJsonSchemaPath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"atmos\"] using ENV variable", "ATMOS_SCHEMAS_ATMOS_MANIFEST", atmosManifestJsonSchemaPath)
		atmosConfig.Schemas["atmos"] = schema.SchemaRegistry{
			Manifest: atmosManifestJsonSchemaPath,
		}
	}

	tfAppendUserAgent := os.Getenv("ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT")
	if len(tfAppendUserAgent) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT", tfAppendUserAgent)
		atmosConfig.Components.Terraform.AppendUserAgent = tfAppendUserAgent
	}

	// Note: ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE and ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE_DIR
	// are handled via Viper bindEnv in setEnv() and populated during Unmarshal, not here.

	listMergeStrategy := os.Getenv("ATMOS_SETTINGS_LIST_MERGE_STRATEGY")
	if len(listMergeStrategy) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_SETTINGS_LIST_MERGE_STRATEGY", listMergeStrategy)
		atmosConfig.Settings.ListMergeStrategy = listMergeStrategy
	}

	versionEnabled := os.Getenv("ATMOS_VERSION_CHECK_ENABLED")
	if len(versionEnabled) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_VERSION_CHECK_ENABLED", versionEnabled)
		enabled, err := strconv.ParseBool(versionEnabled)
		if err != nil {
			log.Warn("Invalid boolean value for ENV variable; using default.", "ATMOS_VERSION_CHECK_ENABLED", versionEnabled)
		} else {
			atmosConfig.Version.Check.Enabled = enabled
		}
	}

	return nil
}

func checkConfig(atmosConfig schema.AtmosConfiguration, isProcessStack bool) error {
	if isProcessStack && len(atmosConfig.Stacks.BasePath) < 1 {
		return errors.New("stack base path must be provided in 'stacks.base_path' config or ATMOS_STACKS_BASE_PATH' ENV variable")
	}

	if isProcessStack && len(atmosConfig.Stacks.IncludedPaths) < 1 {
		return errors.New("at least one path must be provided in 'stacks.included_paths' config or ATMOS_STACKS_INCLUDED_PATHS' ENV variable")
	}

	if len(atmosConfig.Logs.Level) > 0 {
		if _, err := log.ParseLogLevel(atmosConfig.Logs.Level); err != nil {
			// Extract explanation from error message (format: "sentinel\nexplanation").
			errMsg := err.Error()
			parts := strings.SplitN(errMsg, "\n", 2)
			if len(parts) > 1 {
				// Return error with explanation preserved.
				return fmt.Errorf("%w\n%s", log.ErrInvalidLogLevel, parts[1])
			}
			return err
		}
	}

	// Validate version constraint.
	if err := validateVersionConstraint(atmosConfig.Version.Constraint); err != nil {
		return err
	}

	return nil
}

// getVersionEnforcement returns the enforcement level, checking env var override.
func getVersionEnforcement(configEnforcement string) string {
	if envEnforcement := os.Getenv("ATMOS_VERSION_ENFORCEMENT"); envEnforcement != "" { //nolint:forbidigo
		return envEnforcement
	}
	if configEnforcement == "" {
		return "fatal"
	}
	return configEnforcement
}

// buildVersionConstraintError builds the error for unsatisfied version constraint.
func buildVersionConstraintError(constraint schema.VersionConstraint) error {
	builder := errUtils.Build(errUtils.ErrVersionConstraint).
		WithExplanationf("This configuration requires Atmos version %s, but you are running %s",
			constraint.Require, version.Version).
		WithHint("Please upgrade: https://atmos.tools/install").
		WithContext("required", constraint.Require).
		WithContext("current", version.Version).
		WithExitCode(1)

	if constraint.Message != "" {
		builder = builder.WithHint(constraint.Message)
	}

	return builder.Err()
}

// warnVersionConstraint logs a warning for unsatisfied version constraint.
func warnVersionConstraint(constraint schema.VersionConstraint) {
	_ = ui.Warning(fmt.Sprintf(
		"Atmos version constraint not satisfied\n  Required: %s\n  Current:  %s",
		constraint.Require,
		version.Version,
	))
	if constraint.Message != "" {
		_ = ui.Warning(constraint.Message)
	}
}

// validateVersionConstraint validates the current Atmos version against the constraint
// specified in atmos.yaml. Uses the Atmos error builder pattern - no deep exits.
func validateVersionConstraint(constraint schema.VersionConstraint) error {
	if constraint.Require == "" {
		return nil
	}

	enforcement := getVersionEnforcement(constraint.Enforcement)
	if enforcement == "silent" {
		return nil
	}

	satisfied, err := version.ValidateConstraint(constraint.Require)
	if err != nil {
		return errUtils.Build(errors.Join(errUtils.ErrInvalidVersionConstraint, err)).
			WithHint("Please use valid semver constraint syntax").
			WithHint("Reference: https://github.com/hashicorp/go-version").
			WithContext("constraint", constraint.Require).
			WithExitCode(1).
			Err()
	}

	if satisfied {
		return nil
	}

	switch enforcement {
	case "fatal":
		return buildVersionConstraintError(constraint)
	case "warn":
		warnVersionConstraint(constraint)
	}

	return nil
}

const cmdLineArg = "Set using command line argument"

func processCommandLineArgs(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	defer perf.Track(atmosConfig, "config.processCommandLineArgs")()

	if err := setBasePaths(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setTerraformConfig(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setHelmfileConfig(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setPackerConfig(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setStacksConfig(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setFeatureFlags(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setSchemaDirs(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setLoggingConfig(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}
	if err := setSettingsConfig(atmosConfig, configAndStacksInfo); err != nil {
		return err
	}

	return nil
}

func setBasePaths(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.BasePath) > 0 {
		atmosConfig.BasePath = configAndStacksInfo.BasePath
		log.Debug(cmdLineArg, BasePathFlag, configAndStacksInfo.BasePath)
	}
	return nil
}

func setTerraformConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.TerraformCommand) > 0 {
		atmosConfig.Components.Terraform.Command = configAndStacksInfo.TerraformCommand
		log.Debug(cmdLineArg, TerraformCommandFlag, configAndStacksInfo.TerraformCommand)
	}
	if len(configAndStacksInfo.TerraformDir) > 0 {
		atmosConfig.Components.Terraform.BasePath = configAndStacksInfo.TerraformDir
		log.Debug(cmdLineArg, TerraformDirFlag, configAndStacksInfo.TerraformDir)
	}
	return nil
}

func setHelmfileConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.HelmfileCommand) > 0 {
		atmosConfig.Components.Helmfile.Command = configAndStacksInfo.HelmfileCommand
		log.Debug(cmdLineArg, HelmfileCommandFlag, configAndStacksInfo.HelmfileCommand)
	}
	if len(configAndStacksInfo.HelmfileDir) > 0 {
		atmosConfig.Components.Helmfile.BasePath = configAndStacksInfo.HelmfileDir
		log.Debug(cmdLineArg, HelmfileDirFlag, configAndStacksInfo.HelmfileDir)
	}
	return nil
}

func setPackerConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.PackerCommand) > 0 {
		atmosConfig.Components.Packer.Command = configAndStacksInfo.PackerCommand
		log.Debug(cmdLineArg, PackerCommandFlag, configAndStacksInfo.PackerCommand)
	}
	if len(configAndStacksInfo.PackerDir) > 0 {
		atmosConfig.Components.Packer.BasePath = configAndStacksInfo.PackerDir
		log.Debug(cmdLineArg, PackerDirFlag, configAndStacksInfo.PackerDir)
	}
	return nil
}

func setStacksConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.StacksDir) > 0 {
		atmosConfig.Stacks.BasePath = configAndStacksInfo.StacksDir
		log.Debug(cmdLineArg, StackDirFlag, configAndStacksInfo.StacksDir)
	}
	return nil
}

func setFeatureFlags(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.DeployRunInit) > 0 {
		deployRunInitBool, err := strconv.ParseBool(configAndStacksInfo.DeployRunInit)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.DeployRunInit = deployRunInitBool
		log.Debug(cmdLineArg, DeployRunInitFlag, configAndStacksInfo.DeployRunInit)
	}
	if len(configAndStacksInfo.AutoGenerateBackendFile) > 0 {
		autoGenerateBackendFileBool, err := strconv.ParseBool(configAndStacksInfo.AutoGenerateBackendFile)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.AutoGenerateBackendFile = autoGenerateBackendFileBool
		log.Debug(cmdLineArg, AutoGenerateBackendFileFlag, configAndStacksInfo.AutoGenerateBackendFile)
	}
	if len(configAndStacksInfo.WorkflowsDir) > 0 {
		atmosConfig.Workflows.BasePath = configAndStacksInfo.WorkflowsDir
		log.Debug(cmdLineArg, WorkflowDirFlag, configAndStacksInfo.WorkflowsDir)
	}
	if len(configAndStacksInfo.InitRunReconfigure) > 0 {
		initRunReconfigureBool, err := strconv.ParseBool(configAndStacksInfo.InitRunReconfigure)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.InitRunReconfigure = initRunReconfigureBool
		log.Debug(cmdLineArg, InitRunReconfigure, configAndStacksInfo.InitRunReconfigure)
	}
	if len(configAndStacksInfo.InitPassVars) > 0 {
		initPassVarsBool, err := strconv.ParseBool(configAndStacksInfo.InitPassVars)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.Init.PassVars = initPassVarsBool
		log.Debug(cmdLineArg, InitPassVars, configAndStacksInfo.InitPassVars)
	}
	if len(configAndStacksInfo.PlanSkipPlanfile) > 0 {
		planSkipPlanfileBool, err := strconv.ParseBool(configAndStacksInfo.PlanSkipPlanfile)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.Plan.SkipPlanfile = planSkipPlanfileBool
		log.Debug(cmdLineArg, PlanSkipPlanfile, configAndStacksInfo.PlanSkipPlanfile)
	}
	return nil
}

func setSchemaDirs(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.JsonSchemaDir) > 0 {
		atmosConfig.Schemas["jsonschema"] = schema.ResourcePath{BasePath: configAndStacksInfo.JsonSchemaDir}
		log.Debug(cmdLineArg, JsonSchemaDirFlag, configAndStacksInfo.JsonSchemaDir)
	}
	if len(configAndStacksInfo.OpaDir) > 0 {
		atmosConfig.Schemas["opa"] = schema.ResourcePath{BasePath: configAndStacksInfo.OpaDir}
		log.Debug(cmdLineArg, OpaDirFlag, configAndStacksInfo.OpaDir)
	}
	if len(configAndStacksInfo.CueDir) > 0 {
		atmosConfig.Schemas["cue"] = schema.ResourcePath{BasePath: configAndStacksInfo.CueDir}
		log.Debug(cmdLineArg, CueDirFlag, configAndStacksInfo.CueDir)
	}
	if len(configAndStacksInfo.AtmosManifestJsonSchema) > 0 {
		atmosConfig.Schemas["atmos"] = schema.SchemaRegistry{
			Manifest: configAndStacksInfo.AtmosManifestJsonSchema,
		}
		log.Debug(cmdLineArg, AtmosManifestJsonSchemaFlag, configAndStacksInfo.AtmosManifestJsonSchema)
	}
	return nil
}

func setLoggingConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.LogsLevel) > 0 {
		normalizedLevel, err := log.ParseLogLevel(configAndStacksInfo.LogsLevel)
		if err != nil {
			return err
		}
		// Set the normalized log level (title case, with aliases resolved)
		atmosConfig.Logs.Level = string(normalizedLevel)
		log.Debug(cmdLineArg, LogsLevelFlag, atmosConfig.Logs.Level)
	}
	if len(configAndStacksInfo.LogsFile) > 0 {
		atmosConfig.Logs.File = configAndStacksInfo.LogsFile
		log.Debug(cmdLineArg, LogsFileFlag, configAndStacksInfo.LogsFile)
	}
	return nil
}

func setSettingsConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.SettingsListMergeStrategy) > 0 {
		atmosConfig.Settings.ListMergeStrategy = configAndStacksInfo.SettingsListMergeStrategy
		log.Debug(cmdLineArg, SettingsListMergeStrategyFlag, configAndStacksInfo.SettingsListMergeStrategy)
	}

	return nil
}

// processStoreConfig creates a store registry from the provided stores config and assigns it to the atmosConfig.
func processStoreConfig(atmosConfig *schema.AtmosConfiguration) error {
	if len(atmosConfig.StoresConfig) > 0 {
		log.Debug("processStoreConfig", "atmosConfig.StoresConfig", fmt.Sprintf("%v", atmosConfig.StoresConfig))
	}

	storeRegistry, err := store.NewStoreRegistry(&atmosConfig.StoresConfig)
	if err != nil {
		return err
	}
	atmosConfig.Stores = storeRegistry

	return nil
}

// GetContextFromVars creates a context object from the provided variables.
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

// GetContextPrefix calculates context prefix from the context.
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

// ReplaceContextTokens replaces context tokens in the provided pattern and returns a string with all the tokens replaced.
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
		"{attributes}", strings.Join(u.SliceOfInterfacesToSliceOfStrings(context.Attributes), "-"),
	)
	return r.Replace(pattern)
}

// GetStackNameFromContextAndStackNamePattern calculates stack name from the provided context using the provided stack name pattern.
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

// getStackFilePatterns returns a slice of possible file patterns for a given base path.
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

// matchesStackFilePattern checks if a file path matches any of the valid stack file patterns.
func matchesStackFilePattern(filePath, stackName string) bool {
	defer perf.Track(nil, "config.matchesStackFilePattern")()

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
	defer perf.Track(nil, "config.getConfigFilePatterns")()

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

func SearchConfigFile(configPath string, atmosConfig schema.AtmosConfiguration) (string, error) {
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
