package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const foundEnvVarMessage = "Found ENV variable"

// configEnvValue reads only the named environment binding, without allowing global
// Viper defaults, config values, or flags to change an environment-only override.
func configEnvValue(name string) string {
	v := viper.New()
	bindEnv(v, name, name)
	return v.GetString(name)
}

// processEnvVars applies the ATMOS_* environment variable overrides to the Atmos configuration.
func processEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "config.processEnvVars")()

	for _, apply := range []func(*schema.AtmosConfiguration) error{
		processPathEnvVars,
		processTerraformInitEnvVars,
		processTerraformModeEnvVars,
		processTerraformPlanEnvVars,
		processTerraformFlagEnvVars,
		processHelmfileEnvVars,
		processResourceEnvVars,
		processFeatureEnvVars,
	} {
		if err := apply(atmosConfig); err != nil {
			return err
		}
	}

	return nil
}

// setInitEnumEnvVar reads envVar, normalizes it (trim + lowercase), validates it with isValid,
// and stores it in dest. Shared by the init.mode/init.reconfigure/init.upgrade ENV variables so
// each one stays a single call in processEnvVars instead of a repeated read/validate/assign block.
func setInitEnumEnvVar[T ~string](envVar, foundEnvVarMessage string, isValid func(T) bool, sentinel error, dest *T) error {
	value := configEnvValue(envVar)
	if len(value) == 0 {
		return nil
	}
	log.Debug(foundEnvVarMessage, envVar, value)
	normalized, err := normalizeInitEnumValue(value, isValid, sentinel)
	if err != nil {
		return err
	}
	*dest = normalized
	return nil
}

// normalizeConfiguredMocksMode lower-cases and validates components.terraform.mocks.mode as loaded
// from atmos.yaml, so a typo fails at config load (as an invalid ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE
// does) instead of being accepted silently until a --use-mocks lookup runs.
func normalizeConfiguredMocksMode(atmosConfig *schema.AtmosConfiguration) error {
	mode, err := normalizeInitEnumValue(string(atmosConfig.Components.Terraform.Mocks.Mode),
		schema.TerraformMocksMode.IsValid, errUtils.ErrInvalidMocksMode)
	if err != nil {
		return fmt.Errorf("%w (from components.terraform.mocks.mode in atmos.yaml)", err)
	}
	atmosConfig.Components.Terraform.Mocks.Mode = mode
	return nil
}

// normalizeConfiguredToolchainInstall lower-cases and validates toolchain.install as loaded from
// atmos.yaml or ATMOS_TOOLCHAIN_INSTALL (bound through Viper), so a typo fails at config load
// instead of being accepted silently until a tool is needed.
func normalizeConfiguredToolchainInstall(atmosConfig *schema.AtmosConfiguration) error {
	policy, err := normalizeInitEnumValue(string(atmosConfig.Toolchain.Install),
		schema.ToolchainInstall.IsValid, errUtils.ErrInvalidToolchainInstall)
	if err != nil {
		return errUtils.Build(err).
			WithHintf("Set `toolchain.install` (or `ATMOS_TOOLCHAIN_INSTALL`) to one of: %s", toolchainInstallValuesList()).
			WithContext("value", string(atmosConfig.Toolchain.Install)).
			Err()
	}
	atmosConfig.Toolchain.Install = policy
	return nil
}

// toolchainInstallValuesList renders the valid toolchain.install values for hints.
func toolchainInstallValuesList() string {
	values := make([]string, 0, len(schema.ToolchainInstallValues))
	for _, value := range schema.ToolchainInstallValues {
		values = append(values, string(value))
	}
	return strings.Join(values, ", ")
}

// normalizeInitEnumValue trims and lower-cases value, then validates it with isValid, returning
// a wrapped sentinel error (with the original, un-normalized value for readability) when invalid.
func normalizeInitEnumValue[T ~string](value string, isValid func(T) bool, sentinel error) (T, error) {
	normalized := T(strings.ToLower(strings.TrimSpace(value)))
	if !isValid(normalized) {
		return "", fmt.Errorf("%w: %q", sentinel, value)
	}
	return normalized, nil
}

func processPathEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	basePath := configEnvValue("ATMOS_BASE_PATH")
	if len(basePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_BASE_PATH", basePath)
		atmosConfig.BasePath = basePath
		atmosConfig.BasePathSource = basePathSourceRuntime
	}

	vendorBasePath := configEnvValue("ATMOS_VENDOR_BASE_PATH")
	if len(vendorBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_VENDOR_BASE_PATH", vendorBasePath)
		atmosConfig.Vendor.BasePath = vendorBasePath
	}

	stacksBasePath := configEnvValue("ATMOS_STACKS_BASE_PATH")
	if len(stacksBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_BASE_PATH", stacksBasePath)
		atmosConfig.Stacks.BasePath = stacksBasePath
	}

	stacksIncludedPaths := configEnvValue("ATMOS_STACKS_INCLUDED_PATHS")
	if len(stacksIncludedPaths) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_INCLUDED_PATHS", stacksIncludedPaths)
		atmosConfig.Stacks.IncludedPaths = strings.Split(stacksIncludedPaths, ",")
	}

	stacksExcludedPaths := configEnvValue("ATMOS_STACKS_EXCLUDED_PATHS")
	if len(stacksExcludedPaths) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_EXCLUDED_PATHS", stacksExcludedPaths)
		atmosConfig.Stacks.ExcludedPaths = strings.Split(stacksExcludedPaths, ",")
	}

	stacksNamePattern := configEnvValue("ATMOS_STACKS_NAME_PATTERN")
	if len(stacksNamePattern) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_NAME_PATTERN", stacksNamePattern)
		atmosConfig.Stacks.NamePattern = stacksNamePattern
	}

	stacksNameTemplate := configEnvValue("ATMOS_STACKS_NAME_TEMPLATE")
	if len(stacksNameTemplate) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_STACKS_NAME_TEMPLATE", stacksNameTemplate)
		atmosConfig.Stacks.NameTemplate = stacksNameTemplate
	}

	return nil
}

func processTerraformInitEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	componentsTerraformCommand := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_COMMAND")
	if len(componentsTerraformCommand) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_COMMAND", componentsTerraformCommand)
		atmosConfig.Components.Terraform.Command = componentsTerraformCommand
	}

	componentsTerraformBasePath := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH")
	if len(componentsTerraformBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", componentsTerraformBasePath)
		atmosConfig.Components.Terraform.BasePath = componentsTerraformBasePath
	}

	componentsTerraformApplyAutoApprove := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE")
	if len(componentsTerraformApplyAutoApprove) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE", componentsTerraformApplyAutoApprove)
		applyAutoApproveBool, err := strconv.ParseBool(componentsTerraformApplyAutoApprove)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.ApplyAutoApprove = applyAutoApproveBool
	}

	componentsTerraformDeployRunInit := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT")
	if len(componentsTerraformDeployRunInit) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT", componentsTerraformDeployRunInit)
		deployRunInitBool, err := strconv.ParseBool(componentsTerraformDeployRunInit)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.DeployRunInit = deployRunInitBool
	}

	return processTerraformInitFlagsEnv(atmosConfig)
}

func processTerraformInitFlagsEnv(atmosConfig *schema.AtmosConfiguration) error {
	componentsInitRunReconfigure := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE")
	if len(componentsInitRunReconfigure) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE", componentsInitRunReconfigure)
		initRunReconfigureBool, err := strconv.ParseBool(componentsInitRunReconfigure)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.OverrideInitRunReconfigure(initRunReconfigureBool)
	}

	componentsInitPassVars := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_INIT_PASS_VARS")
	if len(componentsInitPassVars) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_INIT_PASS_VARS", componentsInitPassVars)
		initPassVarsBool, err := strconv.ParseBool(componentsInitPassVars)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.Init.PassVars = initPassVarsBool
	}

	return nil
}

func processTerraformModeEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	if err := setInitEnumEnvVar("ATMOS_COMPONENTS_TERRAFORM_INIT_MODE", foundEnvVarMessage,
		schema.TerraformInitMode.IsValid, errUtils.ErrInvalidInitMode, &atmosConfig.Components.Terraform.Init.Mode); err != nil {
		return err
	}

	if err := setInitEnumEnvVar("ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE", foundEnvVarMessage,
		schema.TerraformMocksMode.IsValid, errUtils.ErrInvalidMocksMode, &atmosConfig.Components.Terraform.Mocks.Mode); err != nil {
		return fmt.Errorf("%w (from ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE)", err)
	}

	if err := normalizeConfiguredMocksMode(atmosConfig); err != nil {
		return err
	}

	if err := normalizeConfiguredToolchainInstall(atmosConfig); err != nil {
		return err
	}

	if err := setInitEnumEnvVar("ATMOS_COMPONENTS_TERRAFORM_INIT_RECONFIGURE", foundEnvVarMessage,
		schema.TerraformInitReconfigure.IsValid, errUtils.ErrInvalidInitReconfigure, &atmosConfig.Components.Terraform.Init.Reconfigure); err != nil {
		return err
	}

	if err := setInitEnumEnvVar("ATMOS_COMPONENTS_TERRAFORM_INIT_UPGRADE", foundEnvVarMessage,
		schema.TerraformInitUpgrade.IsValid, errUtils.ErrInvalidInitUpgrade, &atmosConfig.Components.Terraform.Init.Upgrade); err != nil {
		return err
	}

	return nil
}

func processTerraformPlanEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	componentsPlanSkipPlanfile := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_PLAN_SKIP_PLANFILE")
	if len(componentsPlanSkipPlanfile) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_PLAN_SKIP_PLANFILE", componentsPlanSkipPlanfile)
		planSkipPlanfileBool, err := strconv.ParseBool(componentsPlanSkipPlanfile)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.Plan.SkipPlanfile = planSkipPlanfileBool
	}

	componentsTerraformAutoGenerateBackendFile := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE")
	if len(componentsTerraformAutoGenerateBackendFile) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE", componentsTerraformAutoGenerateBackendFile)
		componentsTerraformAutoGenerateBackendFileBool, err := strconv.ParseBool(componentsTerraformAutoGenerateBackendFile)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.AutoGenerateBackendFile = componentsTerraformAutoGenerateBackendFileBool
	}

	return nil
}

func processTerraformFlagEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	componentsTerraformFlagsLockTimeout := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_FLAGS_LOCK_TIMEOUT")
	if len(componentsTerraformFlagsLockTimeout) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_FLAGS_LOCK_TIMEOUT", componentsTerraformFlagsLockTimeout)
		atmosConfig.Components.Terraform.Flags.LockTimeout = componentsTerraformFlagsLockTimeout
	}

	componentsTerraformFlagsLock := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_FLAGS_LOCK")
	if len(componentsTerraformFlagsLock) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_FLAGS_LOCK", componentsTerraformFlagsLock)
		componentsTerraformFlagsLockBool, err := strconv.ParseBool(componentsTerraformFlagsLock)
		if err != nil {
			return fmt.Errorf("%w: ATMOS_COMPONENTS_TERRAFORM_FLAGS_LOCK: %w", errUtils.ErrInvalidConfig, err)
		}
		atmosConfig.Components.Terraform.Flags.Lock = &componentsTerraformFlagsLockBool
	}

	componentsTerraformFlagsParallelism := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_FLAGS_PARALLELISM")
	if len(componentsTerraformFlagsParallelism) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_FLAGS_PARALLELISM", componentsTerraformFlagsParallelism)
		componentsTerraformFlagsParallelismInt, err := strconv.Atoi(componentsTerraformFlagsParallelism)
		if err != nil {
			return fmt.Errorf("%w: ATMOS_COMPONENTS_TERRAFORM_FLAGS_PARALLELISM: %w", errUtils.ErrInvalidConfig, err)
		}
		atmosConfig.Components.Terraform.Flags.Parallelism = &componentsTerraformFlagsParallelismInt
	}

	componentsTerraformFlagsRefresh := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_FLAGS_REFRESH")
	if len(componentsTerraformFlagsRefresh) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_FLAGS_REFRESH", componentsTerraformFlagsRefresh)
		componentsTerraformFlagsRefreshBool, err := strconv.ParseBool(componentsTerraformFlagsRefresh)
		if err != nil {
			return fmt.Errorf("%w: ATMOS_COMPONENTS_TERRAFORM_FLAGS_REFRESH: %w", errUtils.ErrInvalidConfig, err)
		}
		atmosConfig.Components.Terraform.Flags.Refresh = &componentsTerraformFlagsRefreshBool
	}

	componentsTerraformFlagsCompactWarnings := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_FLAGS_COMPACT_WARNINGS")
	if len(componentsTerraformFlagsCompactWarnings) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_FLAGS_COMPACT_WARNINGS", componentsTerraformFlagsCompactWarnings)
		componentsTerraformFlagsCompactWarningsBool, err := strconv.ParseBool(componentsTerraformFlagsCompactWarnings)
		if err != nil {
			return fmt.Errorf("%w: ATMOS_COMPONENTS_TERRAFORM_FLAGS_COMPACT_WARNINGS: %w", errUtils.ErrInvalidConfig, err)
		}
		atmosConfig.Components.Terraform.Flags.CompactWarnings = componentsTerraformFlagsCompactWarningsBool
	}

	return nil
}

func processHelmfileEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	componentsHelmfileCommand := configEnvValue("ATMOS_COMPONENTS_HELMFILE_COMMAND")
	if len(componentsHelmfileCommand) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_COMMAND", componentsHelmfileCommand)
		atmosConfig.Components.Helmfile.Command = componentsHelmfileCommand
	}

	componentsHelmfileBasePath := configEnvValue("ATMOS_COMPONENTS_HELMFILE_BASE_PATH")
	if len(componentsHelmfileBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_BASE_PATH", componentsHelmfileBasePath)
		atmosConfig.Components.Helmfile.BasePath = componentsHelmfileBasePath
	}

	componentsHelmfileUseEKS := configEnvValue("ATMOS_COMPONENTS_HELMFILE_USE_EKS")
	if len(componentsHelmfileUseEKS) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_USE_EKS", componentsHelmfileUseEKS)
		useEKSBool, err := strconv.ParseBool(componentsHelmfileUseEKS)
		if err != nil {
			return err
		}
		atmosConfig.Components.Helmfile.UseEKS = useEKSBool
	}

	componentsHelmfileKubeconfigPath := configEnvValue("ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH")
	if len(componentsHelmfileKubeconfigPath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH", componentsHelmfileKubeconfigPath)
		atmosConfig.Components.Helmfile.KubeconfigPath = componentsHelmfileKubeconfigPath
	}

	componentsHelmfileHelmAwsProfilePattern := configEnvValue("ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN")
	if len(componentsHelmfileHelmAwsProfilePattern) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN", componentsHelmfileHelmAwsProfilePattern)
		atmosConfig.Components.Helmfile.HelmAwsProfilePattern = componentsHelmfileHelmAwsProfilePattern
	}

	componentsHelmfileClusterNamePattern := configEnvValue("ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN")
	if len(componentsHelmfileClusterNamePattern) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN", componentsHelmfileClusterNamePattern)
		atmosConfig.Components.Helmfile.ClusterNamePattern = componentsHelmfileClusterNamePattern
	}

	return nil
}

func processResourceEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	componentsPackerCommand := configEnvValue("ATMOS_COMPONENTS_PACKER_COMMAND")
	if len(componentsPackerCommand) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_PACKER_COMMAND", componentsPackerCommand)
		atmosConfig.Components.Packer.Command = componentsPackerCommand
	}

	componentsPackerBasePath := configEnvValue("ATMOS_COMPONENTS_PACKER_BASE_PATH")
	if len(componentsPackerBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_PACKER_BASE_PATH", componentsPackerBasePath)
		atmosConfig.Components.Packer.BasePath = componentsPackerBasePath
	}

	workflowsBasePath := configEnvValue("ATMOS_WORKFLOWS_BASE_PATH")
	if len(workflowsBasePath) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_WORKFLOWS_BASE_PATH", workflowsBasePath)
		atmosConfig.Workflows.BasePath = workflowsBasePath
	}

	jsonschemaBasePath := configEnvValue("ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH")
	if len(jsonschemaBasePath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"jsonschema\"] using ENV variable", "ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH", jsonschemaBasePath)
		atmosConfig.SetResourcePath("jsonschema", schema.ResourcePath{BasePath: jsonschemaBasePath})
	}

	opaBasePath := configEnvValue("ATMOS_SCHEMAS_OPA_BASE_PATH")
	if len(opaBasePath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"opa\"] using ENV variable", "ATMOS_SCHEMAS_OPA_BASE_PATH", opaBasePath)
		atmosConfig.SetResourcePath("opa", schema.ResourcePath{BasePath: opaBasePath})
	}

	cueBasePath := configEnvValue("ATMOS_SCHEMAS_CUE_BASE_PATH")
	if len(cueBasePath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"cue\"] using ENV variable", "ATMOS_SCHEMAS_CUE_BASE_PATH", cueBasePath)
		atmosConfig.SetResourcePath("cue", schema.ResourcePath{BasePath: cueBasePath})
	}

	atmosManifestJsonSchemaPath := configEnvValue("ATMOS_SCHEMAS_ATMOS_MANIFEST")
	if len(atmosManifestJsonSchemaPath) > 0 {
		log.Debug("Set atmosConfig.Schemas[\"atmos\"] using ENV variable", "ATMOS_SCHEMAS_ATMOS_MANIFEST", atmosManifestJsonSchemaPath)
		atmosConfig.SetSchemaRegistry("atmos", schema.SchemaRegistry{Manifest: atmosManifestJsonSchemaPath})
	}

	tfAppendUserAgent := configEnvValue("ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT")
	if len(tfAppendUserAgent) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT", tfAppendUserAgent)
		atmosConfig.Components.Terraform.AppendUserAgent = tfAppendUserAgent
	}

	// Note: ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE and ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE_DIR
	// are handled via Viper bindEnv in setEnv() and populated during Unmarshal, not here.

	listMergeStrategy := configEnvValue("ATMOS_SETTINGS_LIST_MERGE_STRATEGY")
	if len(listMergeStrategy) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_SETTINGS_LIST_MERGE_STRATEGY", listMergeStrategy)
		atmosConfig.Settings.ListMergeStrategy = listMergeStrategy
	}

	return nil
}

func processFeatureEnvVars(atmosConfig *schema.AtmosConfiguration) error {
	versionEnabled := configEnvValue("ATMOS_VERSION_CHECK_ENABLED")
	if len(versionEnabled) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_VERSION_CHECK_ENABLED", versionEnabled)
		enabled, err := strconv.ParseBool(versionEnabled)
		if err != nil {
			log.Warn("Invalid boolean value for ENV variable; using default.", "ATMOS_VERSION_CHECK_ENABLED", versionEnabled)
		} else {
			atmosConfig.Version.Check.Enabled = enabled
		}
	}

	ciCommentsEnabled := configEnvValue("ATMOS_CI_COMMENTS_ENABLED")
	if len(ciCommentsEnabled) > 0 {
		log.Debug(foundEnvVarMessage, "ATMOS_CI_COMMENTS_ENABLED", ciCommentsEnabled)
		enabled, err := strconv.ParseBool(ciCommentsEnabled)
		if err != nil {
			log.Warn("Invalid boolean value for ENV variable; using default.", "ATMOS_CI_COMMENTS_ENABLED", ciCommentsEnabled)
		} else {
			atmosConfig.CI.Comments.Enabled = &enabled
		}
	}

	return nil
}
