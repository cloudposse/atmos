package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	goversion "github.com/hashicorp/go-version"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	_ "github.com/cloudposse/atmos/pkg/store/providers" // Register the built-in store backends.
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	errStackBasePathMissing      = errors.New("stack base path must be provided in 'stacks.base_path' config or ATMOS_STACKS_BASE_PATH' ENV variable")
	errStackIncludedPathsMissing = errors.New("at least one path must be provided in 'stacks.included_paths' config or ATMOS_STACKS_INCLUDED_PATHS' ENV variable")
	errSpecifiedConfigNotFound   = errors.New("specified config file not found")
	errImportMatchNotFound       = errors.New("failed to find a match for the import")
)

// resolveStackGlobMatches expands p into its candidate glob patterns (template-aware, via
// getStackFilePatterns when p has no extension), resolves matches for each, and distinguishes a
// genuine glob error (invalid pattern syntax, permission denied) from a valid-but-empty pattern,
// which must not abort discovery of the other includeStackPaths entries (cloudposse/atmos#2867).
// Returns (nil, nil) when the pattern is valid but matched nothing -- callers must treat that as
// "continue to the next included_paths entry", not an error. Shared by
// stackPathExcluded preserves physical-stack matching's tolerance of invalid
// exclusion patterns while normal manifest discovery treats them as exclusions.
func stackPathExcluded(patterns []string, path string, excludeInvalid bool) bool {
	for _, pattern := range patterns {
		matched, err := u.PathMatch(pattern, path)
		if err != nil {
			if excludeInvalid {
				return true
			}
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// FindAllStackConfigsInPathsForStack and FindAllStackConfigsInPaths, which were previously
// duplicating this exact pattern-expansion/error-construction logic.
func resolveStackGlobMatches(atmosConfig *schema.AtmosConfiguration, p string) ([]string, error) {
	patterns := []string{p}

	ext := filepath.Ext(p)
	if ext == "" {
		// Get all patterns since filtering is done later.
		patterns = getStackFilePatterns(p, true)
	}

	var allMatches []string
	for _, pattern := range patterns {
		matches, err := u.GetGlobMatches(pattern)
		if err != nil {
			// GetGlobMatches always errors (wrapping ErrFailedToFindImport) when a pattern
			// matches nothing -- e.g. a stacks.included_paths entry whose directory doesn't
			// exist yet. That's expected and must not abort discovery for the other entries in
			// includeStackPaths (cloudposse/atmos#2867: a second, currently-empty
			// included_paths entry was hard-failing the whole lookup, even though an earlier
			// entry already matched real stack manifests). Any other error (invalid pattern
			// syntax, permission denied, etc.) is genuine and must be preserved -- checking
			// every pattern's own error here, instead of only retrying patterns[0] after the
			// loop, so a real error from a LATER pattern is never silently discarded.
			if errors.Is(err, errUtils.ErrFailedToFindImport) {
				continue
			}
			return nil, errUtils.Build(err).
				WithHintf("Verify `stacks.base_path` in `atmos.yaml` points to the correct directory").
				WithHint("Check that the stacks directory exists and contains stack configuration files").
				WithContext("pattern", pattern).
				WithContext("stacks_base_path", atmosConfig.StacksBaseAbsolutePath).
				Err()
		}
		allMatches = append(allMatches, matches...)
	}

	return allMatches, nil
}

// StackConfigFiles contains discovered manifest paths and the physical-stack match mode.
type StackConfigFiles struct {
	AbsolutePaths []string
	RelativePaths []string
	Physical      bool
}

// FindAllStackConfigsInPathsForStack finds all stack manifests in the paths specified by globs for the provided stack.
func FindAllStackConfigsInPathsForStack(atmosConfig *schema.AtmosConfiguration, stack string, includeStackPaths, excludeStackPaths []string) (StackConfigFiles, error) {
	var absolutePaths []string
	var relativePaths []string
	stackIsDir := strings.IndexAny(stack, "/") > 0

	for _, p := range includeStackPaths {
		allMatches, err := resolveStackGlobMatches(atmosConfig, p)
		if err != nil {
			return StackConfigFiles{}, err
		}
		if len(allMatches) == 0 {
			continue
		}

		// Process all matches found
		for _, matchedFileAbsolutePath := range allMatches {
			matchedFileRelativePath := u.TrimBasePathFromPath(atmosConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)

			// Check if the provided stack matches a file in the config folders (excluding the files from `excludeStackPaths`)
			stackMatch := matchesStackFilePattern(filepath.ToSlash(matchedFileAbsolutePath), stack)

			if stackMatch && stackIsDir && !stackPathExcluded(excludeStackPaths, matchedFileAbsolutePath, false) {
				return StackConfigFiles{[]string{matchedFileAbsolutePath}, []string{matchedFileRelativePath}, true}, nil
			}
			include := !stackPathExcluded(excludeStackPaths, matchedFileAbsolutePath, true)

			if include {
				absolutePaths = append(absolutePaths, matchedFileAbsolutePath)
				relativePaths = append(relativePaths, matchedFileRelativePath)
			}
		}
	}

	if len(absolutePaths) == 0 {
		// The paths are backtick-wrapped so the markdown error renderer treats them as a literal
		// code span instead of parsing the "**" glob wildcard (a common included_paths pattern) as
		// emphasis, which otherwise corrupts the rendered path list.
		return StackConfigFiles{}, fmt.Errorf("%w for the provided stack '%s' in the paths `%v`", errUtils.ErrNoStackManifestsFound, stack, includeStackPaths)
	}

	return StackConfigFiles{absolutePaths, relativePaths, false}, nil
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
		allMatches, err := resolveStackGlobMatches(atmosConfig, p)
		if err != nil {
			return nil, nil, err
		}
		if len(allMatches) == 0 {
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

	if len(absolutePaths) == 0 {
		// Backtick-wrapped for the same reason as the ForStack variant above: avoid the markdown
		// renderer misinterpreting "**" glob wildcards in the path list as emphasis.
		return nil, nil, fmt.Errorf("%w in the paths `%v`", errUtils.ErrNoStackManifestsFound, includeStackPaths)
	}

	return absolutePaths, relativePaths, nil
}

func checkConfig(atmosConfig *schema.AtmosConfiguration, isProcessStack bool) error {
	if isProcessStack && len(atmosConfig.Stacks.BasePath) < 1 {
		return errStackBasePathMissing
	}

	if isProcessStack && len(atmosConfig.Stacks.IncludedPaths) < 1 {
		return errStackIncludedPathsMissing
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

func explicitVersionOverride() string {
	return version.ExplicitVersionOverride(os.Args)
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

func buildInvalidVersionConstraintError(constraint schema.VersionConstraint, err error) error {
	return errUtils.Build(errors.Join(errUtils.ErrInvalidVersionConstraint, err)).
		WithHint("Please use valid semver constraint syntax").
		WithHint("Reference: https://github.com/hashicorp/go-version").
		WithContext("constraint", constraint.Require).
		WithExitCode(1).
		Err()
}

// warnVersionConstraint logs a warning for unsatisfied version constraint.
func warnVersionConstraint(constraint schema.VersionConstraint) {
	ui.Warning(fmt.Sprintf(
		"Atmos version constraint not satisfied\n  Required: %s\n  Current:  %s",
		constraint.Require,
		version.Version,
	))
	if constraint.Message != "" {
		ui.Warning(constraint.Message)
	}
}

// warnVersionConstraintOverride logs a warning when an explicit version override
// bypasses a failed or non-evaluable version constraint.
func warnVersionConstraintOverride(constraint schema.VersionConstraint, requestedVersion string, validationErr error) {
	message := "Atmos version constraint not satisfied; explicit override bypasses enforcement"
	keyvals := []interface{}{
		"required", constraint.Require,
		"current", version.Version,
		"override", requestedVersion,
	}
	if validationErr != nil {
		message = "Atmos version constraint could not be evaluated; explicit override bypasses enforcement"
		keyvals = append(keyvals, "error", validationErr)
	}
	if constraint.Message != "" {
		keyvals = append(keyvals, "configured_message", constraint.Message)
	}

	log.Warn(message, keyvals...)
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

	if _, err := goversion.NewConstraint(constraint.Require); err != nil {
		return buildInvalidVersionConstraintError(constraint, fmt.Errorf("invalid version constraint %q: %w", constraint.Require, err))
	}

	requestedVersion := explicitVersionOverride()

	satisfied, err := version.ValidateConstraint(constraint.Require)
	if err != nil {
		if requestedVersion != "" {
			warnVersionConstraintOverride(constraint, requestedVersion, err)
			return nil
		}
		return buildInvalidVersionConstraintError(constraint, err)
	}

	if satisfied {
		return nil
	}

	if requestedVersion != "" {
		warnVersionConstraintOverride(constraint, requestedVersion, nil)
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
		atmosConfig.BasePathSource = basePathSourceRuntime
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
	return setTerraformInitFlags(atmosConfig, configAndStacksInfo)
}

// applyInitEnumFlag normalizes and validates a CLI-flag override (raw, possibly empty) for one of
// the init.mode/init.reconfigure/init.upgrade tri-state settings, storing the result in dest when
// raw is non-empty. Shared by setFeatureFlags so each of the three flags stays a single call.
func setTerraformInitFlags(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.InitRunReconfigure) > 0 {
		initRunReconfigureBool, err := strconv.ParseBool(configAndStacksInfo.InitRunReconfigure)
		if err != nil {
			return err
		}
		atmosConfig.Components.Terraform.OverrideInitRunReconfigure(initRunReconfigureBool)
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
	if err := setInitEnumFlags(atmosConfig, configAndStacksInfo); err != nil {
		return err
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

func setInitEnumFlags(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if err := applyInitEnumFlag(configAndStacksInfo.InitMode, InitModeFlag,
		schema.TerraformInitMode.IsValid, errUtils.ErrInvalidInitMode, &atmosConfig.Components.Terraform.Init.Mode); err != nil {
		return err
	}
	if err := applyInitEnumFlag(configAndStacksInfo.InitReconfigure, InitReconfigureFlag,
		schema.TerraformInitReconfigure.IsValid, errUtils.ErrInvalidInitReconfigure, &atmosConfig.Components.Terraform.Init.Reconfigure); err != nil {
		return err
	}
	if err := applyInitEnumFlag(configAndStacksInfo.InitUpgrade, InitUpgradeFlag,
		schema.TerraformInitUpgrade.IsValid, errUtils.ErrInvalidInitUpgrade, &atmosConfig.Components.Terraform.Init.Upgrade); err != nil {
		return err
	}
	if err := applyInitEnumFlag(configAndStacksInfo.MocksMode, UseMocksFlag,
		schema.TerraformMocksMode.IsValid, errUtils.ErrInvalidMocksMode, &atmosConfig.Components.Terraform.Mocks.Mode); err != nil {
		return err
	}
	return nil
}

func applyInitEnumFlag[T ~string](raw, flagName string, isValid func(T) bool, sentinel error, dest *T) error {
	if len(raw) == 0 {
		return nil
	}
	normalized, err := normalizeInitEnumValue(raw, isValid, sentinel)
	if err != nil {
		return err
	}
	*dest = normalized
	log.Debug(cmdLineArg, flagName, raw)
	return nil
}

// setSchemaDirs applies the schema directory and manifest overrides from the command line to the Atmos configuration.
func setSchemaDirs(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if len(configAndStacksInfo.JsonSchemaDir) > 0 {
		atmosConfig.SetResourcePath("jsonschema", schema.ResourcePath{BasePath: configAndStacksInfo.JsonSchemaDir})
		log.Debug(cmdLineArg, JsonSchemaDirFlag, configAndStacksInfo.JsonSchemaDir)
	}
	if len(configAndStacksInfo.OpaDir) > 0 {
		atmosConfig.SetResourcePath("opa", schema.ResourcePath{BasePath: configAndStacksInfo.OpaDir})
		log.Debug(cmdLineArg, OpaDirFlag, configAndStacksInfo.OpaDir)
	}
	if len(configAndStacksInfo.CueDir) > 0 {
		atmosConfig.SetResourcePath("cue", schema.ResourcePath{BasePath: configAndStacksInfo.CueDir})
		log.Debug(cmdLineArg, CueDirFlag, configAndStacksInfo.CueDir)
	}
	if len(configAndStacksInfo.AtmosManifestJsonSchema) > 0 {
		atmosConfig.SetSchemaRegistry("atmos", schema.SchemaRegistry{Manifest: configAndStacksInfo.AtmosManifestJsonSchema})
		log.Debug(cmdLineArg, AtmosManifestJsonSchemaFlag, configAndStacksInfo.AtmosManifestJsonSchema)
	}
	return nil
}

// setLoggingConfig resolves log color and applies explicit command-line level and file overrides.
// Invalid color or level values return an error instead of silently choosing a default.
func setLoggingConfig(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if err := setLoggingColor(atmosConfig, configAndStacksInfo.LogsColor); err != nil {
		return err
	}
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
		return nil
	}

	// Fallback: command paths that call InitCliConfig directly (e.g. `describe
	// config`) populate ConfigAndStacksInfo with the zero value, so the CLI
	// flag never reaches the assignment above. Scan os.Args ourselves to
	// honor `--settings-list-merge-strategy=...` on those paths, mirroring how
	// setLogConfig handles `--logs-level`.
	if v, ok := parseFlags()["settings-list-merge-strategy"]; ok && v != "" {
		atmosConfig.Settings.ListMergeStrategy = v
		log.Debug(cmdLineArg, SettingsListMergeStrategyFlag, v)
	}

	return nil
}

// processStoreConfig creates a store registry from the provided stores config and assigns it to
// the atmosConfig. A misconfigured individual store no longer fails this (and thus the whole
// config load); store.NewStoreRegistry logs it as a warning and omits it from the registry.
func processStoreConfig(atmosConfig *schema.AtmosConfiguration) error {
	if len(atmosConfig.StoresConfig) > 0 {
		log.Debug("processStoreConfig", "atmosConfig.StoresConfig", fmt.Sprintf("%v", atmosConfig.StoresConfig))
	}

	// Mark secret-by-default backends (e.g. 1Password) as `secret: true` before building the
	// registry, so the secrets subsystem (which reads StoreConfig.Secret) sees them as secret.
	store.ApplySecretDefaults(atmosConfig.StoresConfig)

	storeRegistry, err := store.NewStoreRegistry(&atmosConfig.StoresConfig)
	if err != nil {
		return err
	}
	atmosConfig.Stores = storeRegistry

	return nil
}
