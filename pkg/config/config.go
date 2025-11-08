package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

// InitCliConfig finds and merges CLI configurations in the following order: system dir, home dir, current dir, ENV vars, command-line arguments
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
//
// TODO: Change configAndStacksInfo to pointer.
// Temporarily suppressing gocritic warnings; refactoring InitCliConfig would require extensive changes.
//
//nolint:gocritic
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
	atmosConfig, err := processAtmosConfigs(&configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	// Process the base path specified in the Terraform provider (which calls into the atmos code)
	// This overrides all other atmos base path configs (`atmos.yaml`, ENV var `ATMOS_BASE_PATH`)
	if configAndStacksInfo.AtmosBasePath != "" {
		atmosConfig.BasePath = configAndStacksInfo.AtmosBasePath
	}

	// After unmarshalling, ensure AppendUserAgent is set if still empty
	if atmosConfig.Components.Terraform.AppendUserAgent == "" {
		atmosConfig.Components.Terraform.AppendUserAgent = fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version)
	}

	// Check config
	err = checkConfig(atmosConfig, processStacks)
	if err != nil {
		return atmosConfig, err
	}

	err = AtmosConfigAbsolutePaths(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Set log config BEFORE processing stacks so pre-hooks (including auth) see the correct log level.
	setLogConfig(&atmosConfig)

	if processStacks {
		err = processStackConfigs(&atmosConfig, &configAndStacksInfo, atmosConfig.IncludeStackAbsolutePaths, atmosConfig.ExcludeStackAbsolutePaths)
		if err != nil {
			return atmosConfig, err
		}
	}

	atmosConfig.Initialized = true
	return atmosConfig, nil
}

func setLogConfig(atmosConfig *schema.AtmosConfiguration) {
	// TODO: This is a quick patch to mitigate the issue we can look for better code later
	// Issue: https://linear.app/cloudposse/issue/DEV-3093/create-a-cli-command-core-library
	if os.Getenv("ATMOS_LOGS_LEVEL") != "" {
		atmosConfig.Logs.Level = os.Getenv("ATMOS_LOGS_LEVEL")
	}
	flagKeyValue := parseFlags()
	if v, ok := flagKeyValue["logs-level"]; ok {
		atmosConfig.Logs.Level = v
	}
	if os.Getenv("ATMOS_LOGS_FILE") != "" {
		atmosConfig.Logs.File = os.Getenv("ATMOS_LOGS_FILE")
	}
	if v, ok := flagKeyValue["logs-file"]; ok {
		atmosConfig.Logs.File = v
	}
	if val, ok := flagKeyValue["no-color"]; ok {
		valLower := strings.ToLower(val)
		switch valLower {
		case "true":
			atmosConfig.Settings.Terminal.NoColor = true
			atmosConfig.Settings.Terminal.Color = false
		case "false":
			atmosConfig.Settings.Terminal.NoColor = false
			atmosConfig.Settings.Terminal.Color = true
		}
		// If value is neither "true" nor "false", leave defaults unchanged
	}

	// Handle --pager global flag
	if v, ok := flagKeyValue["pager"]; ok {
		atmosConfig.Settings.Terminal.Pager = v
	}

	// Handle NO_PAGER environment variable (standard CLI convention)
	// Check this after --pager flag so CLI flag takes precedence
	//nolint:forbidigo // NO_PAGER is a standard CLI convention that requires direct env access.
	// We intentionally don't use viper.BindEnv() here because:
	// 1. NO_PAGER uses negative logic (NO_PAGER=true disables pager)
	// 2. Atmos config convention uses positive boolean names (pager: true enables pager)
	// 3. We don't want a configurable "no_pager" field that would confuse the config schema
	// 4. NO_PAGER should remain an environment-only standard, not a config file setting
	if os.Getenv("NO_PAGER") != "" {
		// Check if --pager flag was explicitly provided
		if _, hasPagerFlag := flagKeyValue["pager"]; !hasPagerFlag {
			// NO_PAGER is set and no explicit --pager flag was provided, disable the pager
			atmosConfig.Settings.Terminal.Pager = "false"
		}
	}

	// Configure global logger with the log level from flags/env/config.
	// This ensures auth pre-hooks (executed during processStackConfigs) respect the log level.
	// Parse and convert log level using existing utilities for consistency.
	logLevel, err := log.ParseLogLevel(atmosConfig.Logs.Level)
	if err != nil {
		// Default to Info on parse error (matches ParseLogLevel behavior for empty string).
		logLevel = log.LogLevelInfo
	}
	log.SetLevel(log.ConvertLogLevel(logLevel))
}

// TODO: This function works well, but we should generally avoid implementing manual flag parsing,
// as Cobra typically handles this.

// If there's no alternative, this approach may be necessary.
// However, this TODO serves as a reminder to revisit and verify if a better solution exists.

// Function to manually parse flags with double dash "--" like Cobra.
func parseFlags() map[string]string {
	return parseFlagsFromArgs(os.Args)
}

// parseFlagsFromArgs parses flags from the given args slice.
// This function is exposed for testing purposes.
func parseFlagsFromArgs(args []string) map[string]string {
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Check if the argument starts with '--' (double dash)
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		// Strip the '--' prefix and check if it's followed by a value
		arg = arg[2:]
		switch {
		case strings.Contains(arg, "="):
			// Case like --flag=value
			parts := strings.SplitN(arg, "=", 2)
			flags[parts[0]] = parts[1]
		case i+1 < len(args) && !strings.HasPrefix(args[i+1], "--"):
			// Case like --flag value
			flags[arg] = args[i+1]
			i++ // Skip the next argument as it's the value
		default:
			// Case where flag has no value, e.g., --flag (we set it to "true")
			flags[arg] = "true"
		}
	}
	return flags
}

func processAtmosConfigs(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	atmosConfig, err := LoadConfig(configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.ProcessSchemas()

	// Process ENV vars
	err = processEnvVars(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Process command-line args
	err = processCommandLineArgs(&atmosConfig, configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}

	// Process stores config
	err = processStoreConfig(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}
	return atmosConfig, nil
}

// atmosConfigAbsolutePaths converts paths to absolute paths.
// AtmosConfigAbsolutePaths converts all base paths in the configuration to absolute paths.
// This function sets TerraformDirAbsolutePath, HelmfileDirAbsolutePath, PackerDirAbsolutePath,
// StacksBaseAbsolutePath, IncludeStackAbsolutePaths, and ExcludeStackAbsolutePaths.
func AtmosConfigAbsolutePaths(atmosConfig *schema.AtmosConfiguration) error {
	// Convert stacks base path to an absolute path
	stacksBasePath := u.JoinPath(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return err
	}
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert Terraform dir to an absolute path.
	terraformBasePath := u.JoinPath(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert Helmfile dir to an absolute path.
	helmfileBasePath := u.JoinPath(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	// Convert Packer dir to an absolute path.
	packerBasePath := u.JoinPath(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath)
	packerDirAbsPath, err := filepath.Abs(packerBasePath)
	if err != nil {
		return err
	}
	atmosConfig.PackerDirAbsolutePath = packerDirAbsPath

	return nil
}

func processStackConfigs(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, includeStackAbsPaths, excludeStackAbsPaths []string) error {
	// If the specified stack name is a logical name, find all stack manifests in the provided paths
	stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
		*atmosConfig,
		configAndStacksInfo.Stack,
		includeStackAbsPaths,
		excludeStackAbsPaths,
	)
	if err != nil {
		return err
	}

	if len(stackConfigFilesAbsolutePaths) < 1 {
		j, err := u.ConvertToYAML(includeStackAbsPaths)
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
			"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
			"files or ENV vars.", j)
		return errors.New(errorMessage)
	}

	atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
	atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

	if stackIsPhysicalPath {
		log.Debug("The stack matches the stack manifest",
			"stack", configAndStacksInfo.Stack,
			"manifest", stackConfigFilesRelativePaths[0])
		atmosConfig.StackType = "Directory"
	} else {
		// The stack is a logical name
		atmosConfig.StackType = "Logical"
	}

	return nil
}
