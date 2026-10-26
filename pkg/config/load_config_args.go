package config

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// configSourceDirs bundles the two directory-tracking accumulators threaded through
// mergeFiles/mergeConfigFromDirectories/mergeConfigFromCLIArgs, keeping each function's result
// count under revive's function-result-limit.
type configSourceDirs struct {
	// basePath is the directory of whichever --config/--config-path source declared (or, absent
	// any declaration, the first source that contributed) base_path, for resolveAbsolutePath to
	// anchor a relative base_path against the correct source's directory instead of
	// CliConfigPath's ";"-joined multi-directory string.
	basePath string
	// profilesBasePath is the directory of whichever source declared profiles.base_path, if any,
	// for discoverProfileLocations to resolve it against the correct source's directory, not just
	// the first source's -- cloudposse/atmos#2867.
	profilesBasePath string
}

// mergeConfigFromCLIArgs merges config sources selected via --config/--config-path into v,
// returning the directories that contributed configuration (for CliConfigPath assembly) alongside
// the base_path/profiles.base_path source-directory tracking (see configSourceDirs).
//
// --config files and --config-path directories are folded through the SAME running
// configSourceDirs accumulator, in merge order (files, then directories -- see the comment
// below), so "whichever source declared it last, else the first source overall" holds across both
// flags combined, not just within one.
//
// Split out from loadConfigFromCLIArgs so LoadConfig's main flow can merge the CLI-selected
// config and then fall through into the same profile-loading/edition/final-unmarshal tail
// every other config source uses, instead of returning immediately. Previously, using
// --config/--config-path meant LoadConfig returned right after loadConfigFromCLIArgs' own,
// separate unmarshal -- silently skipping profile loading entirely (--config and --profile
// could not be combined, cloudposse/atmos#2867/#2868 audit finding) along with the
// profiles.base_path/vendor-updater/container-runtime config bridging the shared tail
// performs for every other config source.
func mergeConfigFromCLIArgs(v *viper.Viper, configAndStacksInfo *schema.ConfigAndStacksInfo) ([]string, configSourceDirs, error) {
	log.Debug("loading config from command line arguments")

	configFilesArgs := configAndStacksInfo.AtmosConfigFilesFromArg
	configDirsArgs := configAndStacksInfo.AtmosConfigDirsFromArg
	var configPaths []string
	var dirs configSourceDirs

	// Merge all config from --config files. --config-path directories are always merged AFTER
	// --config files below, regardless of which flag appears later on the command line --
	// documented (website/docs/cli/configuration/configuration.mdx) and intentional, not a bug.
	if len(configFilesArgs) > 0 {
		fileDirs, err := mergeFiles(v, configFilesArgs)
		if err != nil {
			return nil, configSourceDirs{}, err
		}
		dirs = fileDirs
		for _, configFilePath := range configFilesArgs {
			configPaths = append(configPaths, filepath.Dir(configFilePath))
		}
	}

	// Merge config from --config-path directories, continuing the same configSourceDirs fold
	// started above (zero-value accumulator if no --config files were given, so the first
	// --config-path directory becomes the default anchor).
	if len(configDirsArgs) > 0 {
		paths, dirDirs, err := mergeConfigFromDirectories(v, configDirsArgs, dirs)
		if err != nil {
			return nil, configSourceDirs{}, err
		}
		configPaths = append(configPaths, paths...)
		dirs = dirDirs
	}

	// Check if any config files were found from command line arguments
	if len(configPaths) == 0 {
		log.Debug("no config files found from command line arguments")
		return nil, configSourceDirs{}, fmt.Errorf("%w: no config files found from command line arguments (--config or --config-path)", errUtils.ErrAtmosArgConfigNotFound)
	}

	return configPaths, dirs, nil
}

// loadConfigFromCLIArgs handles the loading of configurations provided via --config-path and
// --config, unmarshaling directly into atmosConfig on its own (without profile support or the
// shared-tail config bridging LoadConfig's main flow performs -- see mergeConfigFromCLIArgs).
// Kept as a standalone entry point for direct callers/tests exercising --config in isolation.
func loadConfigFromCLIArgs(v *viper.Viper, configAndStacksInfo *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) error {
	configPaths, dirs, err := mergeConfigFromCLIArgs(v, configAndStacksInfo)
	if err != nil {
		return err
	}

	setEnv(v)

	// Apply the edition pin (if any) before unmarshaling, same as the main
	// LoadConfig flow (this path skips that hook otherwise).
	if err := applyEditionDefaults(v); err != nil {
		return err
	}

	if err := v.Unmarshal(atmosConfig, atmosDecodeHook()); err != nil {
		return err
	}
	extractEnvMapsFromViper(v, atmosConfig)

	// Fix auth.identities after Viper unmarshal (same as in main LoadConfig flow).
	// Viper treats dots in map keys as nested paths, which breaks identity names like "product.usa".
	if err := fixAuthIdentities(v, atmosConfig); err != nil {
		return err
	}

	// Preserve case-sensitive map keys (same as in main LoadConfig flow).
	preserveCaseSensitiveMaps(v, atmosConfig)
	restoreCaseSensitiveEnvMaps(atmosConfig)

	// Apply git root discovery for default base path (same as the main LoadConfig
	// auto-discovery flow, load.go). Without this, a config loaded via --config/
	// --config-path with an empty (or ".") base_path never resolves to the git
	// repository root, breaking component/stack path resolution that the exact
	// same atmos.yaml would get right via plain auto-discovery (cloudposse/atmos#2863).
	if err := applyGitRootBasePath(atmosConfig); err != nil {
		log.Debug("Failed to apply git root base path", "error", err)
		// Don't fail config loading if this step fails, just log it (mirrors load.go).
	}

	atmosConfig.CliConfigPath = connectPaths(configPaths)
	atmosConfig.BasePathConfigDir = dirs.basePath
	atmosConfig.ProfilesBasePathConfigDir = dirs.profilesBasePath
	return nil
}

// trackConfigDirs inspects a single config file's raw content for declared base_path and
// profiles.base_path keys, returning the (possibly updated) basePathConfigDir and
// profilesBasePathConfigDir accumulators for mergeFiles' per-file loop. Split out from
// mergeFiles to keep its cognitive complexity down.
func trackConfigDirs(content []byte, configPath, configDir, basePathConfigDir, profilesBasePathConfigDir string) (string, string, error) {
	declaresBasePath, _, err := importBasePathDeclaration(content)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s: %w", errUtils.ErrMergeConfiguration, configPath, err)
	}
	if declaresBasePath || basePathConfigDir == "" {
		basePathConfigDir = configDir
	}

	declaresProfilesPath, err := declaresProfilesBasePath(content)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s: %w", errUtils.ErrMergeConfiguration, configPath, err)
	}
	if declaresProfilesPath {
		profilesBasePathConfigDir = configDir
	}

	return basePathConfigDir, profilesBasePathConfigDir, nil
}

// mergeFiles merges config files from the provided paths, returning the base_path/
// profiles.base_path source-directory tracking (see configSourceDirs) -- the directory of
// whichever file declared (or, absent any declaration, the first file that contributed)
// base_path, and the directory of the file that declared profiles.base_path (empty if none did).
// Both let callers resolve a relative base_path/profiles.base_path against the correct file's
// directory (cloudposse/atmos#2867) instead of always the first --config file's.
func mergeFiles(v *viper.Viper, configFilePaths []string) (configSourceDirs, error) {
	err := validatedIsFiles(configFilePaths)
	if err != nil {
		return configSourceDirs{}, err
	}
	var dirs configSourceDirs
	for _, configPath := range configFilePaths {
		configDir := filepath.Dir(configPath)
		content, readErr := os.ReadFile(configPath)
		if readErr != nil {
			return configSourceDirs{}, fmt.Errorf("%w: %s: %w", errUtils.ErrReadConfig, configPath, readErr)
		}
		dirs.basePath, dirs.profilesBasePath, err = trackConfigDirs(content, configPath, configDir, dirs.basePath, dirs.profilesBasePath)
		if err != nil {
			return configSourceDirs{}, err
		}
		err := mergeConfigFile(configPath, v)
		if err != nil {
			log.Debug("error loading config file", "path", configPath, "error", err)
			return configSourceDirs{}, err
		}
		log.Debug("config file merged", "path", configPath)
		if err := mergeDefaultImports(configPath, v); err != nil {
			log.Debug("error process imports", "path", configPath, "error", err)
		}
		importConfigDir := dirs.basePath
		if v.GetString("base_path") == "" {
			importConfigDir = configDir
		}
		importBasePathDir, importErr := mergeImports(v, importConfigDir, "", v.GetString(runtimeBasePathOverrideKey))
		if importErr != nil {
			log.Debug("error process imports", "file", configPath, "error", importErr)
		}
		if importBasePathDir != "" {
			dirs.basePath = importBasePathDir
		}
	}
	return dirs, nil
}

// mergeConfigFromDirectories merges config files from the provided directories, continuing the
// configSourceDirs fold passed in (e.g. from a prior mergeFiles call over --config files merged
// earlier -- see mergeConfigFromCLIArgs) so "whichever source declared it last, else the first
// source overall" holds across --config and --config-path combined. Returns the updated
// accumulator alongside the merged directories.
func mergeConfigFromDirectories(v *viper.Viper, dirPaths []string, dirs configSourceDirs) ([]string, configSourceDirs, error) {
	if err := validatedIsDirs(dirPaths); err != nil {
		return nil, configSourceDirs{}, err
	}
	var configPaths []string
	for _, confDirPath := range dirPaths {
		err := mergeConfig(v, confDirPath, CliConfigFileName, true)
		if err != nil {
			log.Debug("Failed to find atmos config", "path", confDirPath, "error", err)
			switch err.(type) {
			case viper.ConfigFileNotFoundError:
				log.Debug("Failed to found atmos config", "file", filepath.Join(confDirPath, CliConfigFileName))
			default:
				return nil, configSourceDirs{}, err
			}
			err = mergeConfig(v, confDirPath, DotCliConfigFileName, true)
			if err != nil {
				log.Debug("Failed to found .atmos config", "path", filepath.Join(confDirPath, CliConfigFileName), "error", err)
				return nil, configSourceDirs{}, fmt.Errorf("%w: %s", errUtils.ErrAtmosFilesDirConfigNotFound, confDirPath)
			}
			log.Debug(".atmos config file merged", "path", v.ConfigFileUsed())
		} else {
			log.Debug("atmos config file merged", "path", v.ConfigFileUsed())
		}
		configPaths = append(configPaths, confDirPath)

		usedFilePath := v.ConfigFileUsed()
		content, readErr := os.ReadFile(usedFilePath)
		if readErr != nil {
			log.Debug("failed to read config file for base_path/profiles.base_path tracking", "path", usedFilePath, "error", readErr)
			continue
		}
		dirs.basePath, dirs.profilesBasePath, err = trackConfigDirs(content, usedFilePath, confDirPath, dirs.basePath, dirs.profilesBasePath)
		if err != nil {
			return nil, configSourceDirs{}, err
		}
	}
	return configPaths, dirs, nil
}

func validatedIsDirs(dirPaths []string) error {
	for _, dirPath := range dirPaths {
		if dirPath == "" {
			return fmt.Errorf("%w: --config-path requires a non-empty directory path", errUtils.ErrEmptyConfigPath)
		}
		stat, err := os.Stat(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug("--config-path directory not found", "path", dirPath)
				return fmt.Errorf("%w: --config-path directory '%s' does not exist", errUtils.ErrAtmosDirConfigNotFound, dirPath)
			}
			// Other stat errors (permission denied, etc.)
			return fmt.Errorf("cannot access --config-path directory '%s': %w", dirPath, err)
		}
		if !stat.IsDir() {
			log.Debug("--config-path expected directory found file", "path", dirPath)
			return fmt.Errorf("%w: --config-path requires a directory but found a file at '%s'", errUtils.ErrAtmosDirConfigNotFound, dirPath)
		}
	}
	return nil
}

func validatedIsFiles(files []string) error {
	for _, filePath := range files {
		if filePath == "" {
			return fmt.Errorf("%w: --config requires a non-empty file path", errUtils.ErrEmptyConfigFile)
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug("--config file not found", "path", filePath)
				return fmt.Errorf("%w: --config file '%s' does not exist", errUtils.ErrFileNotFound, filePath)
			}
			// Other stat errors (permission denied, etc.)
			return fmt.Errorf("%w: cannot access --config file '%s': %v", errUtils.ErrFileAccessDenied, filePath, err)
		}
		if stat.IsDir() {
			log.Debug("--config expected file found directory", "path", filePath)
			return errUtils.ErrExpectedFile
		}
	}
	return nil
}

func connectPaths(paths []string) string {
	if len(paths) == 1 {
		return paths[0]
	}
	var result string
	for _, path := range paths {
		if path == "" {
			continue
		}
		result += path + ";"
	}
	return result
}
