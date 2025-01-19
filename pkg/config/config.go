package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/mitchellh/go-homedir"
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
	// atmosConfig is loaded from the following locations (from lower to higher priority):
	// 1. If ATMOS_CLI_CONFIG_PATH is defined, check only there
	// 2. If ATMOS_CLI_CONFIG_PATH is not defined, proceed with other paths
	//    - Check system directory (optional)
	//    - Check user-specific configuration:
	//      - If XDG_CONFIG_HOME is defined, use it; otherwise fallback to ~/.config/atmos
	//      - Check current directory
	//    - If Terraform provider specified a path
	// 3. If no config is found in any of the above locations, use the default config
	// Check if no imports are defined
	var atmosConfig schema.AtmosConfiguration
	var err error
	configFound := false

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)

	// Default configuration values
	v.SetDefault("components.helmfile.use_eks", true)
	v.SetDefault("components.terraform.append_user_agent", fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))
	v.SetDefault("settings.inject_github_token", true)

	// 1. If ATMOS_CLI_CONFIG_PATH is defined, check only there
	if atmosCliConfigPathEnv := os.Getenv("ATMOS_CLI_CONFIG_PATH"); atmosCliConfigPathEnv != "" {
		u.LogTrace(atmosConfig, fmt.Sprintf("Found ENV var ATMOS_CLI_CONFIG_PATH=%s", atmosCliConfigPathEnv))
		configFile := filepath.Join(atmosCliConfigPathEnv, CliConfigFileName)
		found, configPath, err := processConfigFile(atmosConfig, configFile, v)
		if err != nil {
			return atmosConfig, err
		}
		if !found {
			// If we want to error out if config not found in ATMOS_CLI_CONFIG_PATH
			return atmosConfig, fmt.Errorf("config not found in ATMOS_CLI_CONFIG_PATH: %s", configFile)
		} else {
			configFound = true
			atmosConfig.CliConfigPath = configPath
		}

		// Since ATMOS_CLI_CONFIG_PATH is to be the first and only check, we skip other paths if found
		// If not found, we still skip other paths as per the requirement.
	} else {
		// 2. If ATMOS_CLI_CONFIG_PATH is not defined, proceed with other paths
		//    - Check system directory (optional)
		configFilePathSystem := ""
		if runtime.GOOS == "windows" {
			appDataDir := os.Getenv(WindowsAppDataEnvVar)
			if len(appDataDir) > 0 {
				configFilePathSystem = appDataDir
			}
		} else {
			configFilePathSystem = SystemDirConfigFilePath
		}

		if len(configFilePathSystem) > 0 {
			configFile := filepath.Join(configFilePathSystem, CliConfigFileName)
			found, configPath, err := processConfigFile(atmosConfig, configFile, v)
			if err != nil {
				return atmosConfig, err
			}
			if found {
				configFound = true
				atmosConfig.CliConfigPath = configPath
			}
		}

		// 3. Check user-specific configuration:
		//    If XDG_CONFIG_HOME is defined, use it; otherwise fallback to ~/.config/atmos
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		var userConfigDir string
		if xdgConfigHome != "" {
			userConfigDir = filepath.Join(xdgConfigHome, "atmos")
		} else {
			homeDir, err := homedir.Dir()
			if err != nil {
				return atmosConfig, err
			}
			userConfigDir = filepath.Join(homeDir, ".config", "atmos")
		}

		userConfigFile := filepath.Join(userConfigDir, CliConfigFileName)
		found, configPath, err := processConfigFile(atmosConfig, userConfigFile, v)
		if err != nil {
			return atmosConfig, err
		}
		if found {
			configFound = true
			atmosConfig.CliConfigPath = configPath
		}

		// 4. Check current directory
		configFilePathCwd, err := os.Getwd()
		if err != nil {
			return atmosConfig, err
		}
		configFileCwd := filepath.Join(configFilePathCwd, CliConfigFileName)
		found, configPath, err = processConfigFile(atmosConfig, configFileCwd, v)
		if err != nil {
			return atmosConfig, err
		}
		if found {
			configFound = true
			atmosConfig.CliConfigPath = configPath
		}

		// 5. If Terraform provider specified a path
		if configAndStacksInfo.AtmosCliConfigPath != "" {
			configFileTfProvider := filepath.Join(configAndStacksInfo.AtmosCliConfigPath, CliConfigFileName)
			found, configPath, err := processConfigFile(atmosConfig, configFileTfProvider, v)
			if err != nil {
				return atmosConfig, err
			}
			if found {
				configFound = true
				atmosConfig.CliConfigPath = configPath
			}
		}
	}

	if !configFound {
		// Use default config if no config was found in any location
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

		j, err := json.Marshal(defaultCliConfig)
		if err != nil {
			return atmosConfig, err
		}

		reader := bytes.NewReader(j)
		err = v.MergeConfig(reader)
		if err != nil {
			return atmosConfig, err
		}
	}
	// We want the editorconfig color by default to be true
	atmosConfig.Validate.EditorConfig.Color = true
	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err = v.Unmarshal(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	err = processEnvVars(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Check if no imports are defined
	if len(atmosConfig.Import) == 0 {
		basePath, err := filepath.Abs(atmosConfig.BasePath)
		if err != nil {
			return atmosConfig, err
		}
		// Check for an `atmos.d` directory and load the configs if found
		atmosDPath := filepath.Join(basePath, "atmos.d")
		// Ensure the joined path doesn't escape the intended directory
		if !strings.HasPrefix(atmosDPath, basePath) {
			u.LogWarning(atmosConfig, "invalid atmos.d path: attempted directory traversal")
		}
		_, err = os.Stat(atmosDPath)
		if err == nil {
			atmosConfig.Import = []string{"atmos.d/**/*.yaml", "atmos.d/**/*.yml"}
		} else if !os.IsNotExist(err) {
			return atmosConfig, err // Handle unexpected errors
		}
		// Check for `.atmos.d` directory if `.atmos.d` directory is not found
		atmosDPath = filepath.Join(atmosDPath, ".atmos.d")
		_, err = os.Stat(atmosDPath)
		if err == nil {
			cliImport := []string{".atmos.d/**/*.yaml", ".atmos.d/**/*.yml"}
			atmosConfig.Import = append(atmosConfig.Import, cliImport...)
		} else if !os.IsNotExist(err) {
			return atmosConfig, err // Handle unexpected errors
		}

	}
	// Process imports if any
	if len(atmosConfig.Import) > 0 {
		err = processImports(atmosConfig, v)
		if err != nil {
			return atmosConfig, err
		}

		// Re-unmarshal the merged configuration into atmosConfig
		err = v.Unmarshal(&atmosConfig)
		if err != nil {
			return atmosConfig, err
		}
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
			u.LogTrace(atmosConfig, fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
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
			u.LogWarning(atmosConfig, fmt.Sprintf("error closing file '"+configPath+"'. "+err.Error()))
		}
	}(reader)

	err = v.MergeConfig(reader)
	if err != nil {
		return false, "", err
	}

	return true, configPath, nil
}
func processImports(atmosConfig schema.AtmosConfiguration, v *viper.Viper) error {
	for _, importPath := range atmosConfig.Import {
		if importPath == "" {
			continue
		}

		var resolvedPaths []string
		var err error

		if strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://") {
			// Handle remote URLs
			// Validate the URL before downloading
			parsedURL, err := url.Parse(importPath)
			if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
				u.LogWarning(atmosConfig, fmt.Sprintf("unsupported URL '%s': %v", importPath, err))
				continue
			}

			tempDir, tempFile, err := downloadRemoteConfig(importPath)
			if err != nil {
				u.LogWarning(atmosConfig, fmt.Sprintf("failed to download remote config '%s': %v", importPath, err))
				continue
			}
			resolvedPaths = []string{tempFile}
			defer os.RemoveAll(tempDir)

		} else {

			ext := filepath.Ext(importPath)

			basePath, err := filepath.Abs(atmosConfig.BasePath)
			if err != nil {
				return err
			}
			if ext != "" {
				imp := filepath.Join(basePath, importPath)
				resolvedPaths, err = u.GetGlobMatches(imp)
				if err != nil {
					u.LogWarning(atmosConfig, fmt.Sprintf("failed to resolve import path '%s': %v", imp, err))
					continue
				}
			} else {
				impYaml := filepath.Join(basePath, importPath+".yaml")
				impYml := filepath.Join(basePath, importPath+".yml")
				// ensure the joined path doesn't escape the intended directory
				if !strings.HasPrefix(impYaml, basePath) || !strings.HasPrefix(impYml, basePath) {
					return fmt.Errorf("invalid import path: attempted directory traversal")
				}
				resolvedPathYaml, errYaml := u.GetGlobMatches(impYaml)
				resolvedPathsYml, errYml := u.GetGlobMatches(impYml)
				if errYaml != nil && errYml != nil {
					u.LogWarning(atmosConfig, fmt.Sprintf("failed to resolve import path '%s': %v", importPath, err))
					continue
				}
				resolvedPaths = append(resolvedPaths, resolvedPathYaml...)
				resolvedPaths = append(resolvedPaths, resolvedPathsYml...)

			}
		}
		// print the resolved paths
		u.LogTrace(atmosConfig, fmt.Sprintf("Resolved import paths: %v", resolvedPaths))
		for _, path := range resolvedPaths {
			// Process each configuration file
			_, _, err = processConfigFile(atmosConfig, path, v)
			if err != nil {
				// Log the error but continue processing other files
				u.LogWarning(atmosConfig, fmt.Sprintf("failed to merge configuration from '%s': %v", path, err))
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
	tempFile := filepath.Join(tempDir, "atmos.yaml")
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
