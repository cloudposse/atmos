package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"
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

	// Handle deprecated settings
	handleDeprecatedSettings(&atmosConfig)

	// Check config
	err = checkConfig(atmosConfig, processStacks)
	if err != nil {
		return atmosConfig, err
	}

	err = atmosConfigAbsolutePaths(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	if processStacks {
		err = processStackConfigs(&atmosConfig, &configAndStacksInfo, atmosConfig.IncludeStackAbsolutePaths, atmosConfig.ExcludeStackAbsolutePaths)
		if err != nil {
			return atmosConfig, err
		}
	}
	setLogConfig(&atmosConfig)

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
	if flagKeyValue, ok := flagKeyValue["no-color"]; ok || flagKeyValue == "true" {
		atmosConfig.Settings.Terminal.NoColor = true
	} else if flagKeyValue == "false" {
		atmosConfig.Settings.Terminal.NoColor = false
	}
}

// TODO: This function works well, but we should generally avoid implementing manual flag parsing,
// as Cobra typically handles this.

// If there's no alternative, this approach may be necessary.
// However, this TODO serves as a reminder to revisit and verify if a better solution exists.

// Function to manually parse flags with double dash "--" like Cobra.
func parseFlags() map[string]string {
	args := os.Args
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

// handleDeprecatedSettings handles deprecated configuration fields and migrates them to new locations.
func handleDeprecatedSettings(cfg *schema.AtmosConfiguration) {
	// Handle deprecated no_color
	//nolint:staticcheck // SA1019: NoColor is deprecated but we need to check it for migration
	if cfg.Settings.Terminal.NoColor {
		u.NotifyDeprecatedField("settings.terminal.no_color", "settings.terminal.color")
		if cfg.Settings.Terminal.Color == nil {
			color := !cfg.Settings.Terminal.NoColor
			cfg.Settings.Terminal.Color = &color
		}
	}

	// Handle deprecated docs settings
	//nolint:staticcheck // SA1019: Docs.MaxWidth is deprecated but we need to check it for migration
	if cfg.Docs.MaxWidth > 0 && cfg.Settings.Terminal.MaxWidth == 0 {
		cfg.Settings.Terminal.MaxWidth = cfg.Docs.MaxWidth
		u.NotifyDeprecatedField("settings.docs.max-width", "settings.terminal.max_width")
	}

	//nolint:staticcheck // SA1019: Docs.Pagination is deprecated but we need to check it for migration
	if cfg.Docs.Pagination && cfg.Settings.Terminal.Pager == "" {
		cfg.Settings.Terminal.Pager = "on"
		u.NotifyDeprecatedField("settings.docs.pagination", "settings.terminal.pager")
	}
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
func atmosConfigAbsolutePaths(atmosConfig *schema.AtmosConfiguration) error {
	// Convert stacks base path to an absolute path
	stacksBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return err
	}
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert Terraform dir to an absolute path.
	terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert Helmfile dir to an absolute path.
	helmfileBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	// Convert Packer dir to an absolute path.
	packerBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath)
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
