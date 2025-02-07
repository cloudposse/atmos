package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	NotFound = errors.New("\n'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars." +
		"\nYou can download a sample config and adapt it to your requirements from " +
		"https://raw.githubusercontent.com/cloudposse/atmos/main/examples/quick-start-advanced/atmos.yaml")

	defaultCliConfig = schema.AtmosConfiguration{
		Default:  true,
		BasePath: ".",
		Stacks: schema.Stacks{
			BasePath:    "stacks",
			NamePattern: "{tenant}-{environment}-{stage}",
			IncludedPaths: []string{
				"orgs/**/*",
			},
			ExcludedPaths: []string{
				"**/_defaults.yaml",
			},
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath:                "components/terraform",
				ApplyAutoApprove:        false,
				DeployRunInit:           true,
				InitRunReconfigure:      true,
				AutoGenerateBackendFile: true,
				AppendUserAgent:         fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version),
			},
			Helmfile: schema.Helmfile{
				BasePath:              "components/helmfile",
				KubeconfigPath:        "",
				HelmAwsProfilePattern: "{namespace}-{tenant}-gbl-{stage}-helm",
				ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
				UseEKS:                true,
			},
		},
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
			Terminal: schema.Terminal{
				MaxWidth: templates.GetTerminalWidth(),
				Pager:    true,
				Colors:   true,
				Unicode:  true,
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled:                true,
					Formatter:              "terminal",
					Theme:                  "dracula",
					HighlightedOutputPager: true,
					LineNumbers:            true,
					Wrap:                   false,
				},
			},
		},
		Workflows: schema.Workflows{
			BasePath: "stacks/workflows",
		},
		Logs: schema.Logs{
			File:  "/dev/stderr",
			Level: "Info",
		},
		Schemas: schema.Schemas{
			JsonSchema: schema.JsonSchema{
				BasePath: "stacks/schemas/jsonschema",
			},
			Opa: schema.Opa{
				BasePath: "stacks/schemas/opa",
			},
		},
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled:     true,
					Datasources: make(map[string]schema.TemplatesSettingsGomplateDatasource),
				},
			},
		},
		Initialized: true,
		Version: schema.Version{
			Check: schema.VersionCheck{
				Enabled:   true,
				Timeout:   1000,
				Frequency: "daily",
			},
		},
	}
)

// InitCliConfig finds and merges CLI configurations in the following order: system dir, home dir, current dir, ENV vars, command-line arguments
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
	configLoader := NewConfigLoader()
	atmosConfig, err := configLoader.LoadConfig(configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	if len(configLoader.AtmosConfigPaths) == 0 {
		logsLevelEnvVar := os.Getenv("ATMOS_LOGS_LEVEL")
		if logsLevelEnvVar == u.LogLevelDebug || logsLevelEnvVar == u.LogLevelTrace {
			u.PrintMessageInColor("'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars.\n"+
				"Refer to https://atmos.tools/cli/configuration for details on how to configure 'atmos.yaml'.\n"+
				"Using the default CLI config:\n\n", theme.Colors.Info)

			err = u.PrintAsYAMLToFileDescriptor(atmosConfig, defaultCliConfig)
			if err != nil {
				return atmosConfig, err
			}
			fmt.Println()
		}
		// Load Atmos Schema Defaults
		j, err := json.Marshal(defaultCliConfig)
		if err != nil {
			return atmosConfig, err
		}

		reader := bytes.NewReader(j)
		err = configLoader.viper.MergeConfig(reader)
		if err != nil {
			return atmosConfig, err
		}
	}
	atmosConfig.CliConfigPath = ConnectPaths(configLoader.AtmosConfigPaths)
	basePath, err := configLoader.BasePathComputing(configAndStacksInfo)
	if err != nil {
		return atmosConfig, fmt.Errorf("failed to compute base path: %v", err)
	}
	atmosConfig.BasePath = basePath
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
	err = checkConfig(atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Convert stacks base path to absolute path
	stacksBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile dir to absolute path
	helmfileBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	if processStacks {
		// If the specified stack name is a logical name, find all stack manifests in the provided paths
		stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
			atmosConfig,
			configAndStacksInfo.Stack,
			includeStackAbsPaths,
			excludeStackAbsPaths,
		)
		if err != nil {
			return atmosConfig, err
		}

		if len(stackConfigFilesAbsolutePaths) < 1 {
			j, err := u.ConvertToYAML(includeStackAbsPaths)
			if err != nil {
				return atmosConfig, err
			}
			errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
				"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
				"files or ENV vars.", j)
			return atmosConfig, errors.New(errorMessage)
		}

		atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
		atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

		if stackIsPhysicalPath {
			u.LogTrace(fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
				configAndStacksInfo.Stack,
				stackConfigFilesRelativePaths[0]),
			)
			atmosConfig.StackType = "Directory"
		} else {
			// The stack is a logical name
			atmosConfig.StackType = "Logical"
		}
	}

	atmosConfig.Initialized = true
	return atmosConfig, nil
}

// https://github.com/NCAR/go-figure
// https://github.com/spf13/viper/issues/181
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func processConfigFile(
	atmosConfig schema.AtmosConfiguration,
	path string,
	v *viper.Viper,
) (bool, string, error) {
	// Check if the config file exists
	configPath, fileExists := u.SearchConfigFile(path)
	if !fileExists {
		return false, "", nil
	}
	reader, err := os.Open(configPath)
	if err != nil {
		return false, "", err
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			u.LogWarning(fmt.Sprintf("error closing file '%s'. %v", configPath, err))
		}
	}(reader)

	err = v.MergeConfig(reader)
	if err != nil {
		return false, "", err
	}

	return true, configPath, nil
}
