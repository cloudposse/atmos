package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ExecuteRegistryPullCmd executes `Registry list` commands
func ExecuteRegistryListCmd(cmd *cobra.Command, args []string) error {
	err := ExecuteRegistryListCommand(cmd, args)
	if err != nil {
		return err
	}
	return nil
}
func ExecuteRegistryListCommand(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}
	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}
	// Check `vendor.yaml`
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(atmosConfig, cfg.AtmosRegistryConfigFileName, true)
	if err != nil {
		return err
	}
	fmt.Println(vendorConfig)
	fmt.Println(vendorConfigExists)
	fmt.Println(foundVendorConfigFile)

	return nil
}
func ReadAndProcessRegistryConfigFile(
	atmosConfig schema.AtmosConfiguration,
	vendorConfigFile string,
	checkGlobalConfig bool,
) (schema.AtmosVendorConfig, bool, string, error) {
	var vendorConfig schema.AtmosVendorConfig

	// Initialize empty sources slice
	vendorConfig.Spec.Sources = []schema.AtmosVendorSource{}

	var vendorConfigFileExists bool
	var foundRegistryConfigFile string

	// Check if vendor config is specified in atmos.yaml
	if checkGlobalConfig && atmosConfig.Registry.BasePath != "" {
		if !filepath.IsAbs(atmosConfig.Registry.BasePath) {
			foundRegistryConfigFile = filepath.Join(atmosConfig.BasePath, atmosConfig.Registry.BasePath)
		} else {
			foundRegistryConfigFile = atmosConfig.Registry.BasePath
		}
	} else {
		// Path is not defined in atmos.yaml, proceed with existing logic
		var fileExists bool
		foundRegistryConfigFile, fileExists = u.SearchConfigFile(vendorConfigFile)

		if !fileExists {
			// Look for the vendoring manifest in the directory pointed to by the `base_path` setting in `atmos.yaml`
			pathToVendorConfig := filepath.Join(atmosConfig.BasePath, vendorConfigFile)
			foundRegistryConfigFile, fileExists = u.SearchConfigFile(pathToVendorConfig)

			if !fileExists {
				vendorConfigFileExists = false
				u.LogWarning(atmosConfig, fmt.Sprintf("Vendor config file '%s' does not exist. Proceeding without vendor configurations", pathToVendorConfig))
				return vendorConfig, vendorConfigFileExists, "", nil
			}
		}
	}

	// Check if it's a directory
	fileInfo, err := os.Stat(foundRegistryConfigFile)
	if err != nil {
		return vendorConfig, false, "", err
	}

	var configFiles []string
	if fileInfo.IsDir() {
		matches, err := doublestar.Glob(os.DirFS(foundRegistryConfigFile), "*.{yaml,yml}")
		if err != nil {
			return vendorConfig, false, "", err
		}
		for _, match := range matches {
			configFiles = append(configFiles, filepath.Join(foundRegistryConfigFile, match))
		}
		sort.Strings(configFiles)
		if len(configFiles) == 0 {
			return vendorConfig, false, "", fmt.Errorf("no YAML configuration files found in directory '%s'", foundRegistryConfigFile)
		}
	} else {
		configFiles = []string{foundRegistryConfigFile}
	}

	// Process all config files
	var mergedSources []schema.AtmosVendorSource
	var mergedImports []string
	sourceMap := make(map[string]bool) // Track unique sources by component name
	importMap := make(map[string]bool) // Track unique imports

	for _, configFile := range configFiles {
		var currentConfig schema.AtmosVendorConfig
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			return vendorConfig, false, "", err
		}

		err = yaml.Unmarshal(yamlFile, &currentConfig)
		if err != nil {
			return vendorConfig, false, "", err
		}

		// Merge sources, checking for duplicates
		for _, source := range currentConfig.Spec.Sources {
			if source.Component != "" {
				if sourceMap[source.Component] {
					return vendorConfig, false, "", fmt.Errorf("duplicate component '%s' found in config file '%s'",
						source.Component, configFile)
				}
				sourceMap[source.Component] = true
			}
			mergedSources = append(mergedSources, source)
		}

		// Merge imports, checking for duplicates
		for _, imp := range currentConfig.Spec.Imports {
			if importMap[imp] {
				continue // Skip duplicate imports
			}
			importMap[imp] = true
			mergedImports = append(mergedImports, imp)
		}
	}

	// Create final merged config
	vendorConfig.Spec.Sources = mergedSources
	vendorConfig.Spec.Imports = mergedImports
	vendorConfigFileExists = true

	return vendorConfig, vendorConfigFileExists, foundRegistryConfigFile, nil
}
