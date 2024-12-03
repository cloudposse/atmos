package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/go-getter"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	NotFound = errors.New("\n'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars." +
		"\nYou can download a sample config and adapt it to your requirements from " +
		"https://raw.githubusercontent.com/cloudposse/atmos/main/examples/quick-start-advanced/atmos.yaml")

	defaultCliConfig = schema.CliConfiguration{
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
		Workflows: schema.Workflows{
			BasePath: "stacks/workflows",
		},
		Logs: schema.Logs{
			File:  "/dev/stdout",
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
	}
)

// InitCliConfig finds and merges CLI configurations in the following order: system dir, home dir, current dir, ENV vars, command-line arguments
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.CliConfiguration, error) {
	// cliConfig is loaded from the following locations (from lower to higher priority):
	// system dir (`/usr/local/etc/atmos` on Linux, `%LOCALAPPDATA%/atmos` on Windows)
	// home dir (~/.atmos)
	// current directory
	// ENV vars
	// Command-line arguments

	var cliConfig schema.CliConfiguration
	var err error
	configFound := false
	var found bool

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)

	// Default configuration values
	v.SetDefault("components.helmfile.use_eks", true)
	v.SetDefault("components.terraform.append_user_agent", fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))

	// Process config in system folder
	configFilePath1 := ""

	// https://pureinfotech.com/list-environment-variables-windows-10/
	// https://docs.microsoft.com/en-us/windows/deployment/usmt/usmt-recognized-environment-variables
	// https://softwareengineering.stackexchange.com/questions/299869/where-is-the-appropriate-place-to-put-application-configuration-files-for-each-p
	// https://stackoverflow.com/questions/37946282/why-does-appdata-in-windows-7-seemingly-points-to-wrong-folder
	if runtime.GOOS == "windows" {
		appDataDir := os.Getenv(WindowsAppDataEnvVar)
		if len(appDataDir) > 0 {
			configFilePath1 = appDataDir
		}
	} else {
		configFilePath1 = SystemDirConfigFilePath
	}

	if len(configFilePath1) > 0 {
		configFile1 := filepath.Join(configFilePath1, CliConfigFileName)
		found, err = processConfigFile(cliConfig, configFile1, v)
		if err != nil {
			return cliConfig, err
		}
		if found {
			configFound = true
		}
	}

	// Process config in user's HOME dir
	configFilePath2, err := homedir.Dir()
	if err != nil {
		return cliConfig, err
	}
	configFile2 := filepath.Join(configFilePath2, ".atmos", CliConfigFileName)
	found, err = processConfigFile(cliConfig, configFile2, v)
	if err != nil {
		return cliConfig, err
	}
	if found {
		configFound = true
	}

	// Process config in the current dir
	configFilePath3, err := os.Getwd()
	if err != nil {
		return cliConfig, err
	}
	configFile3 := filepath.Join(configFilePath3, CliConfigFileName)
	found, err = processConfigFile(cliConfig, configFile3, v)
	if err != nil {
		return cliConfig, err
	}
	if found {
		configFound = true
	}

	// Process config from the path in ENV var `ATMOS_CLI_CONFIG_PATH`
	configFilePath4 := os.Getenv("ATMOS_CLI_CONFIG_PATH")
	if len(configFilePath4) > 0 {
		u.LogTrace(cliConfig, fmt.Sprintf("Found ENV var ATMOS_CLI_CONFIG_PATH=%s", configFilePath4))
		configFile4 := filepath.Join(configFilePath4, CliConfigFileName)
		found, err = processConfigFile(cliConfig, configFile4, v)
		if err != nil {
			return cliConfig, err
		}
		if found {
			configFound = true
		}
	}

	// Process config from the path specified in the Terraform provider (which calls into the atmos code)
	if configAndStacksInfo.AtmosCliConfigPath != "" {
		configFilePath5 := configAndStacksInfo.AtmosCliConfigPath
		if len(configFilePath5) > 0 {
			configFile5 := filepath.Join(configFilePath5, CliConfigFileName)
			found, err = processConfigFile(cliConfig, configFile5, v)
			if err != nil {
				return cliConfig, err
			}
			if found {
				configFound = true
			}
		}
	}

	if !configFound {
		// If `atmos.yaml` not found, use the default config
		// Set `ATMOS_LOGS_LEVEL` ENV var to "Debug" to see the message about Atmos using the default CLI config
		logsLevelEnvVar := os.Getenv("ATMOS_LOGS_LEVEL")
		if logsLevelEnvVar == u.LogLevelDebug || logsLevelEnvVar == u.LogLevelTrace {
			u.PrintMessageInColor("'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars.\n"+
				"Refer to https://atmos.tools/cli/configuration for details on how to configure 'atmos.yaml'.\n"+
				"Using the default CLI config:\n\n", color.New(color.FgCyan))

			err = u.PrintAsYAMLToFileDescriptor(cliConfig, defaultCliConfig)
			if err != nil {
				return cliConfig, err
			}
			fmt.Println()
		}

		j, err := json.Marshal(defaultCliConfig)
		if err != nil {
			return cliConfig, err
		}

		reader := bytes.NewReader(j)
		err = v.MergeConfig(reader)
		if err != nil {
			return cliConfig, err
		}
	}

	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err = v.Unmarshal(&cliConfig)
	if err != nil {
		return cliConfig,
			err
	}
	// Process ENV vars
	err = processEnvVars(&cliConfig)
	if err != nil {
		return cliConfig, err
	}
	// Check if no imports are defined
	if len(cliConfig.Import) == 0 {
		basePath, err := filepath.Abs(cliConfig.BasePath)
		if err != nil {
			return cliConfig, err
		}
		// Check for an `atmos.d` directory and load the configs if found
		atmosDPath := filepath.Join(basePath, "atmos.d")
		// Ensure the joined path doesn't escape the intended directory
		if !strings.HasPrefix(atmosDPath, basePath) {
			return cliConfig, fmt.Errorf("invalid atmos.d path: attempted directory traversal")
		}

		_, err = os.Stat(atmosDPath)
		if err == nil {
			cliConfig.Import = []string{"atmos.d/**/*.yaml"}
		} else if !os.IsNotExist(err) {
			return cliConfig, err // Handle unexpected errors
		}
	}
	// Process imports if any
	if len(cliConfig.Import) > 0 {
		err = processImports(cliConfig, v)
		if err != nil {
			return cliConfig, err
		}

		// Re-unmarshal the merged configuration into cliConfig
		err = v.Unmarshal(&cliConfig)
		if err != nil {
			return cliConfig, err
		}
	}

	// Process command-line args
	err = processCommandLineArgs(&cliConfig, configAndStacksInfo)
	if err != nil {
		return cliConfig, err
	}

	// Process the base path specified in the Terraform provider (which calls into the atmos code)
	// This overrides all other atmos base path configs (`atmos.yaml`, ENV var `ATMOS_BASE_PATH`)
	if configAndStacksInfo.AtmosBasePath != "" {
		cliConfig.BasePath = configAndStacksInfo.AtmosBasePath
	}

	// After unmarshalling, ensure AppendUserAgent is set if still empty
	if cliConfig.Components.Terraform.AppendUserAgent == "" {
		cliConfig.Components.Terraform.AppendUserAgent = fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version)
	}

	// Check config
	err = checkConfig(cliConfig)
	if err != nil {
		return cliConfig, err
	}

	// Convert stacks base path to absolute path
	stacksBasePath := filepath.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return cliConfig, err
	}
	cliConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, cliConfig.Stacks.IncludedPaths)
	if err != nil {
		return cliConfig, err
	}
	cliConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, cliConfig.Stacks.ExcludedPaths)
	if err != nil {
		return cliConfig, err
	}
	cliConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformBasePath := filepath.Join(cliConfig.BasePath, cliConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return cliConfig, err
	}
	cliConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile dir to absolute path
	helmfileBasePath := filepath.Join(cliConfig.BasePath, cliConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return cliConfig, err
	}
	cliConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	if processStacks {
		// If the specified stack name is a logical name, find all stack manifests in the provided paths
		stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
			cliConfig,
			configAndStacksInfo.Stack,
			includeStackAbsPaths,
			excludeStackAbsPaths,
		)

		if err != nil {
			return cliConfig, err
		}

		if len(stackConfigFilesAbsolutePaths) < 1 {
			j, err := u.ConvertToYAML(includeStackAbsPaths)
			if err != nil {
				return cliConfig, err
			}
			errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
				"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
				"files or ENV vars.", j)
			return cliConfig, errors.New(errorMessage)
		}

		cliConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
		cliConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

		if stackIsPhysicalPath {
			u.LogTrace(cliConfig, fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
				configAndStacksInfo.Stack,
				stackConfigFilesRelativePaths[0]),
			)
			cliConfig.StackType = "Directory"
		} else {
			// The stack is a logical name
			cliConfig.StackType = "Logical"
		}
	}

	cliConfig.Initialized = true
	return cliConfig, nil
}

// https://github.com/NCAR/go-figure
// https://github.com/spf13/viper/issues/181
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func processConfigFile(
	cliConfig schema.CliConfiguration,
	path string,
	v *viper.Viper,
) (bool, error) {
	// Check if the config file exists
	configPath, fileExists := u.SearchConfigFile(path)
	if !fileExists {
		return false, nil
	}
	reader, err := os.Open(configPath)
	if err != nil {
		return false, err
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			u.LogWarning(cliConfig, fmt.Sprintf("error closing file '"+configPath+"'. "+err.Error()))
		}
	}(reader)

	err = v.MergeConfig(reader)
	if err != nil {
		return false, err
	}

	return true, nil
}
func processImports(cliConfig schema.CliConfiguration, v *viper.Viper) error {
	for _, importPath := range cliConfig.Import {
		if importPath == "" {
			continue
		}

		var resolvedPaths []string
		var err error

		if strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://") {
			// Handle remote URLs
			tempDir, tempFile, err := downloadRemoteConfig(importPath)
			if err != nil {
				u.LogWarning(cliConfig, fmt.Sprintf("Warning: failed to download remote config '%s': %v", importPath, err))
				continue
			}
			resolvedPaths = []string{tempFile}
			defer os.RemoveAll(tempDir)

		} else {
			impWithExt := importPath
			ext := filepath.Ext(importPath)
			if ext == "" {
				ext = ".yaml"
				impWithExt = importPath + ext
			}
			basePath, err := filepath.Abs(cliConfig.BasePath)
			if err != nil {
				return err
			}
			imp := filepath.Join(basePath, impWithExt)
			// ensure the joined path doesn't escape the intended directory
			if !strings.HasPrefix(imp, basePath) {
				return fmt.Errorf("invalid import path: attempted directory traversal")
			}
			resolvedPaths, err = u.GetGlobMatches(imp)
			if err != nil {
				u.LogWarning(cliConfig, fmt.Sprintf("Warning: failed to resolve import path '%s': %v", impWithExt, err))
				continue
			}
		}
		// print the resolved paths
		u.LogTrace(cliConfig, fmt.Sprintf("Resolved import paths: %v", resolvedPaths))
		for _, path := range resolvedPaths {
			// Process each configuration file
			_, err = processConfigFile(cliConfig, path, v)
			if err != nil {
				// Log the error but continue processing other files
				u.LogWarning(cliConfig, fmt.Sprintf("Warning: failed to merge configuration from '%s': %v", path, err))
				continue
			}
		}
	}
	return nil
}

func downloadRemoteConfig(url string) (string, string, error) {
	tempDir, err := os.MkdirTemp("", "atmos-import-*")
	if err != nil {
		return "", "", err
	}
	tempFile := filepath.Join(tempDir, "config.yaml")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := &getter.Client{
		Ctx:  ctx,
		Src:  url,
		Dst:  tempFile,
		Mode: getter.ClientModeFile,
	}
	err = client.Get()
	if err != nil {
		os.RemoveAll(tempDir)
		return "", "", fmt.Errorf("failed to download remote config: %w", err)
	}
	return tempDir, tempFile, nil
}
