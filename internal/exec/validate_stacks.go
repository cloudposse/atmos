package exec

import (
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:embed schemas/atmos/atmos-manifest/1.0/atmos-manifest.json
var atmosManifestDefault embed.FS

const atmosManifestDefaultFileName = "schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"

// ExecuteValidateStacksCmd executes `validate stacks` command
func ExecuteValidateStacksCmd(cmd *cobra.Command, args []string) error {
	// Initialize spinner
	message := "Validating Atmos Stacks..."
	p := NewSpinner(message)
	spinnerDone := make(chan struct{})
	// Run spinner in a goroutine
	RunSpinner(p, spinnerDone, message)
	// Ensure spinner is stopped before returning
	defer StopSpinner(p, spinnerDone)

	// Process CLI arguments
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	schemasAtmosManifestFlag, err := flags.GetString("schemas-atmos-manifest")
	if err != nil {
		return err
	}

	if schemasAtmosManifestFlag != "" {
		atmosConfig.Schemas.Atmos.Manifest = schemasAtmosManifestFlag
	}

	err = ValidateStacks(atmosConfig)
	return err
}

// ValidateStacks validates Atmos stack configuration
func ValidateStacks(atmosConfig schema.AtmosConfiguration) error {
	var validationErrorMessages []string

	// 1. Process top-level stack manifests and detect duplicate components in the same stack
	stacksMap, _, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return err
	}

	terraformComponentStackMap, err := createComponentStackMap(atmosConfig, stacksMap, cfg.TerraformSectionName)
	if err != nil {
		return err
	}

	errorList, err := checkComponentStackMap(terraformComponentStackMap)
	if err != nil {
		return err
	}
	validationErrorMessages = append(validationErrorMessages, errorList...)

	helmfileComponentStackMap, err := createComponentStackMap(atmosConfig, stacksMap, cfg.HelmfileSectionName)
	if err != nil {
		return err
	}

	errorList, err = checkComponentStackMap(helmfileComponentStackMap)
	if err != nil {
		return err
	}
	validationErrorMessages = append(validationErrorMessages, errorList...)

	// 2. Check all YAML stack manifests defined in the infrastructure
	// It will check YAML syntax and all the Atmos sections defined in the manifests

	// Check if the Atmos manifest JSON Schema is configured and the file exists
	// The path to the Atmos manifest JSON Schema can be absolute path or a path relative to the `base_path` setting in `atmos.yaml`
	var atmosManifestJsonSchemaFilePath string
	atmosManifestJsonSchemaFileAbsPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Schemas.Atmos.Manifest)

	if atmosConfig.Schemas.Atmos.Manifest == "" {
		// If the validation schema location is not specified, use the embedded one
		f, err := getEmbeddedSchemaPath()
		if err != nil {
			return err
		}
		atmosConfig.Schemas.Atmos.Manifest = f
		u.LogTrace(fmt.Sprintf("Atmos JSON Schema is not configured. Using the default embedded schema"))
	} else if u.FileExists(atmosConfig.Schemas.Atmos.Manifest) {
		atmosManifestJsonSchemaFilePath = atmosConfig.Schemas.Atmos.Manifest
	} else if u.FileExists(atmosManifestJsonSchemaFileAbsPath) {
		atmosManifestJsonSchemaFilePath = atmosManifestJsonSchemaFileAbsPath
	} else if u.IsURL(atmosConfig.Schemas.Atmos.Manifest) {
		atmosManifestJsonSchemaFilePath, err = downloadSchemaFromURL(atmosConfig)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("Schema file '%s' not found. Configure via:\n"+
			"1. 'schemas.atmos.manifest' in atmos.yaml\n"+
			"2. ATMOS_SCHEMAS_ATMOS_MANIFEST env var\n"+
			"3. --schemas-atmos-manifest flag\n\n"+
			"Accepts: absolute path, path relative to base_path, or URL",
			atmosConfig.Schemas.Atmos.Manifest,
		)
	}

	// Include (process and validate) all YAML files in the `stacks` folder in all subfolders
	includedPaths := []string{"**/*"}
	// Don't exclude any YAML files for validation except template files
	excludedPaths := []string{
		// Exclude template files from validation since they may contain invalid YAML before being rendered
		"**/*.tmpl",
		"**/*.yaml.tmpl",
		"**/*.yml.tmpl",
	}

	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(atmosConfig.StacksBaseAbsolutePath, includedPaths)
	if err != nil {
		return err
	}

	stackConfigFilesAbsolutePaths, _, err := cfg.FindAllStackConfigsInPaths(atmosConfig, includeStackAbsPaths, excludedPaths)
	if err != nil {
		return err
	}

	u.LogDebug(fmt.Sprintf("Validating all YAML files in the '%s' folder and all subfolders (excluding template files)\n",
		filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)))

	for _, filePath := range stackConfigFilesAbsolutePaths {
		stackConfig, importsConfig, _, _, _, err := ProcessYAMLConfigFile(
			atmosConfig,
			atmosConfig.StacksBaseAbsolutePath,
			filePath,
			map[string]map[string]any{},
			nil,
			false,
			false,
			false,
			false,
			map[string]any{},
			map[string]any{},
			atmosManifestJsonSchemaFilePath,
		)
		if err != nil {
			validationErrorMessages = append(validationErrorMessages, err.Error())
		}

		// Process and validate the stack manifest
		_, err = ProcessStackConfig(
			atmosConfig,
			atmosConfig.StacksBaseAbsolutePath,
			atmosConfig.TerraformDirAbsolutePath,
			atmosConfig.HelmfileDirAbsolutePath,
			filePath,
			stackConfig,
			false,
			true,
			"",
			map[string]map[string][]string{},
			importsConfig,
			false,
		)
		if err != nil {
			validationErrorMessages = append(validationErrorMessages, err.Error())
		}
	}

	if len(validationErrorMessages) > 0 {
		return errors.New(strings.Join(validationErrorMessages, "\n\n"))
	}

	return nil
}

func createComponentStackMap(
	atmosConfig schema.AtmosConfiguration,
	stacksMap map[string]any,
	componentType string,
) (map[string]map[string][]string, error) {
	var varsSection map[string]any
	var metadataSection map[string]any
	var settingsSection map[string]any
	var envSection map[string]any
	var providersSection map[string]any
	var overridesSection map[string]any
	var backendSection map[string]any
	var backendTypeSection string
	var stackName string
	var err error
	terraformComponentStackMap := make(map[string]map[string][]string)

	for stackManifest, stackSection := range stacksMap {
		if componentsSection, ok := stackSection.(map[string]any)[cfg.ComponentsSectionName].(map[string]any); ok {
			if terraformSection, ok := componentsSection[componentType].(map[string]any); ok {
				for componentName, compSection := range terraformSection {
					componentSection, ok := compSection.(map[string]any)

					if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); !ok {
						metadataSection = map[string]any{}
					}

					// Don't check abstract components (they are never provisioned)
					if IsComponentAbstract(metadataSection) {
						continue
					}

					if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
						varsSection = map[string]any{}
					}

					if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
						settingsSection = map[string]any{}
					}

					if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
						envSection = map[string]any{}
					}

					if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
						providersSection = map[string]any{}
					}

					if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
						overridesSection = map[string]any{}
					}

					if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
						backendSection = map[string]any{}
					}

					if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
						backendTypeSection = ""
					}

					configAndStacksInfo := schema.ConfigAndStacksInfo{
						ComponentFromArg:          componentName,
						Stack:                     stackName,
						ComponentMetadataSection:  metadataSection,
						ComponentVarsSection:      varsSection,
						ComponentSettingsSection:  settingsSection,
						ComponentEnvSection:       envSection,
						ComponentProvidersSection: providersSection,
						ComponentOverridesSection: overridesSection,
						ComponentBackendSection:   backendSection,
						ComponentBackendType:      backendTypeSection,
						ComponentSection: map[string]any{
							cfg.VarsSectionName:        varsSection,
							cfg.MetadataSectionName:    metadataSection,
							cfg.SettingsSectionName:    settingsSection,
							cfg.EnvSectionName:         envSection,
							cfg.ProvidersSectionName:   providersSection,
							cfg.OverridesSectionName:   overridesSection,
							cfg.BackendSectionName:     backendSection,
							cfg.BackendTypeSectionName: backendTypeSection,
						},
					}

					// Find Atmos stack name
					if atmosConfig.Stacks.NameTemplate != "" {
						stackName, err = ProcessTmpl("validate-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
						if err != nil {
							return nil, err
						}
					} else {
						context := cfg.GetContextFromVars(varsSection)
						configAndStacksInfo.Context = context
						stackName, err = cfg.GetContextPrefix(stackManifest, context, GetStackNamePattern(atmosConfig), stackManifest)
						if err != nil {
							return nil, err
						}
					}

					_, ok = terraformComponentStackMap[componentName]
					if !ok {
						terraformComponentStackMap[componentName] = make(map[string][]string)
					}
					terraformComponentStackMap[componentName][stackName] = append(terraformComponentStackMap[componentName][stackName], stackManifest)
				}
			}
		}
	}

	return terraformComponentStackMap, nil
}

func checkComponentStackMap(componentStackMap map[string]map[string][]string) ([]string, error) {
	var res []string

	for componentName, componentSection := range componentStackMap {
		for stackName, stackManifests := range componentSection {
			if len(stackManifests) > 1 {
				// We have the same Atmos component in the same stack configured (or imported) in more than one stack manifest files
				// Check if the component configs are the same (deep-equal) in those stack manifests.
				// If the configs are different, add it to the errors
				var componentConfigs []map[string]any
				for _, stackManifestName := range stackManifests {
					componentConfig, err := ExecuteDescribeComponent(componentName, stackManifestName, true, true, nil)
					if err != nil {
						return nil, err
					}

					// Hide the sections that should not be compared
					componentConfig["atmos_cli_config"] = nil
					componentConfig["atmos_stack"] = nil
					componentConfig["stack"] = nil
					componentConfig["atmos_stack_file"] = nil
					componentConfig["atmos_manifest"] = nil
					componentConfig["sources"] = nil
					componentConfig["imports"] = nil
					componentConfig["deps_all"] = nil
					componentConfig["deps"] = nil

					componentConfigs = append(componentConfigs, componentConfig)
				}

				componentConfigsEqual := true

				for i := 0; i < len(componentConfigs)-1; i++ {
					if !reflect.DeepEqual(componentConfigs[i], componentConfigs[i+1]) {
						componentConfigsEqual = false
						break
					}
				}

				if !componentConfigsEqual {
					var m1 string
					for _, stackManifestName := range stackManifests {
						m1 = m1 + "\n" + fmt.Sprintf("- atmos describe component %s -s %s", componentName, stackManifestName)
					}

					m := fmt.Sprintf("The Atmos component '%[1]s' in the stack '%[2]s' is defined in more than one top-level stack manifest file: %[3]s.\n\n"+
						"The component configurations in the stack manifests are different.\n\n"+
						"To check and compare the component configurations in the stack manifests, run the following commands: %[4]s\n\n"+
						"You can use the '--file' flag to write the results of the above commands to files (refer to https://atmos.tools/cli/commands/describe/component).\n"+
						"You can then use the Linux 'diff' command to compare the files line by line and show the differences (refer to https://man7.org/linux/man-pages/man1/diff.1.html)\n\n"+
						"When searching for the component '%[1]s' in the stack '%[2]s', Atmos can't decide which stack "+
						"manifest file to use to get configuration for the component.\n"+
						"This is a stack misconfiguration.\n\n"+
						"Consider the following solutions to fix the issue:\n"+
						"- Ensure that the same instance of the Atmos '%[1]s' component in the stack '%[2]s' is only defined once (in one YAML stack manifest file)\n"+
						"- When defining multiple instances of the same component in the stack, ensure each has a unique name\n"+
						"- Use multiple-inheritance to combine multiple configurations together (refer to https://atmos.tools/core-concepts/stacks/inheritance)\n\n",
						componentName,
						stackName,
						strings.Join(stackManifests, ", "),
						m1,
					)

					res = append(res, m)
				}
			}
		}
	}

	return res, nil
}

// downloadSchemaFromURL downloads the Atmos JSON Schema file from the provided URL
func downloadSchemaFromURL(atmosConfig schema.AtmosConfiguration) (string, error) {
	manifestURL := atmosConfig.Schemas.Atmos.Manifest

	parsedURL, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL '%s': %w", manifestURL, err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme '%s' for schema manifest", parsedURL.Scheme)
	}

	tempDir := os.TempDir()
	fileName, err := u.GetFileNameFromURL(manifestURL)
	if err != nil || fileName == "" {
		return "", fmt.Errorf("failed to get the file name from the URL '%s': %w", manifestURL, err)
	}

	atmosManifestJsonSchemaFilePath := filepath.Join(tempDir, fileName)

	if err = GoGetterGet(&atmosConfig, manifestURL, atmosManifestJsonSchemaFilePath, getter.ClientModeFile, time.Second*30); err != nil {
		return "", fmt.Errorf("failed to download the Atmos JSON Schema file '%s' from the URL '%s': %w", fileName, manifestURL, err)
	}

	return atmosManifestJsonSchemaFilePath, nil
}

func getEmbeddedSchemaPath() (string, error) {
	embedded, err := fs.ReadFile(atmosManifestDefault, atmosManifestDefaultFileName)
	if err != nil {
		return "", err
	}

	tempDir := os.TempDir()
	atmosManifestJsonSchemaFilePath := filepath.Join(tempDir, atmosManifestDefaultFileName)

	err = u.EnsureDir(atmosManifestJsonSchemaFilePath)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(atmosManifestJsonSchemaFilePath, embedded, 0o644)
	if err != nil {
		return "", err
	}

	return atmosManifestJsonSchemaFilePath, nil
}
