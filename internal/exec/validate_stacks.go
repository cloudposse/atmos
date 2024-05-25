package exec

import (
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteValidateStacksCmd executes `validate stacks` command
func ExecuteValidateStacksCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	schemasAtmosManifestFlag, err := flags.GetString("schemas-atmos-manifest")
	if err != nil {
		return err
	}

	if schemasAtmosManifestFlag != "" {
		cliConfig.Schemas.Atmos.Manifest = schemasAtmosManifestFlag
	}

	var validationErrorMessages []string

	// 1. Process top-level stack manifests and detect duplicate components in the same stack
	stacksMap, _, err := FindStacksMap(cliConfig, false)
	if err != nil {
		return err
	}

	terraformComponentStackMap, err := createComponentStackMap(cliConfig, stacksMap, cfg.TerraformSectionName)
	if err != nil {
		return err
	}

	errorList, err := checkComponentStackMap(terraformComponentStackMap)
	if err != nil {
		return err
	}
	validationErrorMessages = append(validationErrorMessages, errorList...)

	helmfileComponentStackMap, err := createComponentStackMap(cliConfig, stacksMap, cfg.HelmfileSectionName)
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

	if cliConfig.Schemas.Atmos.Manifest != "" {
		atmosManifestJsonSchemaFileAbsPath := path.Join(cliConfig.BasePath, cliConfig.Schemas.Atmos.Manifest)

		if u.FileExists(cliConfig.Schemas.Atmos.Manifest) {
			atmosManifestJsonSchemaFilePath = cliConfig.Schemas.Atmos.Manifest
		} else if u.FileExists(atmosManifestJsonSchemaFileAbsPath) {
			atmosManifestJsonSchemaFilePath = atmosManifestJsonSchemaFileAbsPath
		} else {
			return fmt.Errorf("the Atmos JSON Schema file '%s' does not exist.\n"+
				"It can be configured in the 'schemas.atmos.manifest' section in 'atmos.yaml', or provided using the 'ATMOS_SCHEMAS_ATMOS_MANIFEST' "+
				"ENV variable or '--schemas-atmos-manifest' command line argument.\n"+
				"The path to the schema file should be an absolute path or a path relative to the 'base_path' setting in 'atmos.yaml'.",
				cliConfig.Schemas.Atmos.Manifest)
		}
	}

	// Include (process and validate) all YAML files in the `stacks` folder in all subfolders
	includedPaths := []string{"**/*"}
	// Don't exclude any YAML files for validation
	excludedPaths := []string{}
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(cliConfig.StacksBaseAbsolutePath, includedPaths)
	if err != nil {
		return err
	}

	stackConfigFilesAbsolutePaths, _, err := cfg.FindAllStackConfigsInPaths(cliConfig, includeStackAbsPaths, excludedPaths)
	if err != nil {
		return err
	}

	u.LogDebug(cliConfig, fmt.Sprintf("Validating all YAML files in the '%s' folder and all subfolders\n",
		path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath)))

	for _, filePath := range stackConfigFilesAbsolutePaths {
		stackConfig, importsConfig, _, err := s.ProcessYAMLConfigFile(
			cliConfig,
			cliConfig.StacksBaseAbsolutePath,
			filePath,
			map[string]map[any]any{},
			nil,
			false,
			false,
			false,
			false,
			map[any]any{},
			map[any]any{},
			atmosManifestJsonSchemaFilePath,
		)
		if err != nil {
			validationErrorMessages = append(validationErrorMessages, err.Error())
		}

		// Process and validate the stack manifest
		_, err = s.ProcessStackConfig(
			cliConfig,
			cliConfig.StacksBaseAbsolutePath,
			cliConfig.TerraformDirAbsolutePath,
			cliConfig.HelmfileDirAbsolutePath,
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
	cliConfig schema.CliConfiguration,
	stacksMap map[string]any,
	componentType string,
) (map[string]map[string][]string, error) {
	var varsSection map[any]any
	var metadataSection map[any]any
	var settingsSection map[any]any
	var envSection map[any]any
	var providersSection map[any]any
	var overridesSection map[any]any
	var backendSection map[any]any
	var backendTypeSection string
	var stackName string
	var err error
	terraformComponentStackMap := make(map[string]map[string][]string)

	for stackManifest, stackSection := range stacksMap {
		if componentsSection, ok := stackSection.(map[any]any)[cfg.ComponentsSectionName].(map[string]any); ok {

			// Terraform components
			if terraformSection, ok := componentsSection[componentType].(map[string]any); ok {
				for componentName, compSection := range terraformSection {
					componentSection, ok := compSection.(map[string]any)

					if varsSection, ok = componentSection[cfg.VarsSectionName].(map[any]any); !ok {
						varsSection = map[any]any{}
					}

					if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[any]any); !ok {
						metadataSection = map[any]any{}
					}

					if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[any]any); !ok {
						settingsSection = map[any]any{}
					}

					if envSection, ok = componentSection[cfg.EnvSectionName].(map[any]any); !ok {
						envSection = map[any]any{}
					}

					if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[any]any); !ok {
						providersSection = map[any]any{}
					}

					if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[any]any); !ok {
						overridesSection = map[any]any{}
					}

					if backendSection, ok = componentSection[cfg.BackendSectionName].(map[any]any); !ok {
						backendSection = map[any]any{}
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
					if cliConfig.Stacks.NameTemplate != "" {
						stackName, err = u.ProcessTmpl("validate-stacks-name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
						if err != nil {
							return nil, err
						}
					} else {
						context := cfg.GetContextFromVars(varsSection)
						configAndStacksInfo.Context = context
						stackName, err = cfg.GetContextPrefix(stackManifest, context, GetStackNamePattern(cliConfig), stackManifest)
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
				m := fmt.Sprintf("the Atmos component '%s' in the stack '%s' is defined in more than one top-level stack manifest files: %s.\n"+
					"Atmos can't decide which stack manifest to use to get configuration for the component in the stack.\n"+
					"This is a stack misconfiguration.",
					componentName,
					stackName,
					strings.Join(stackManifests, ", "))
				res = append(res, m)
			}
		}
	}

	return res, nil
}
