package exec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// TerraformConfigKey is the key used in componentInfo maps to store terraform configuration.
	terraformConfigKey = "terraform_config"
)

// ProcessComponentConfig processes component config sections.
func ProcessComponentConfig(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	stack string,
	stacksMap map[string]any,
	componentType string,
	component string,
	authManager auth.AuthManager,
) error {
	defer perf.Track(nil, "exec.ProcessComponentConfig")()

	var stackSection map[string]any
	var componentsSection map[string]any
	var componentTypeSection map[string]any
	var componentSection map[string]any
	var componentVarsSection map[string]any
	var componentSettingsSection map[string]any
	var componentOverridesSection map[string]any
	var componentProvidersSection map[string]any
	var componentHooksSection map[string]any
	var componentImportsSection []string
	var componentEnvSection map[string]any
	var componentAuthSection map[string]any
	var componentBackendSection map[string]any
	var componentBackendType string
	var command string
	var componentInheritanceChain []string
	var ok bool

	if len(stack) == 0 {
		return errors.New("stack must be provided and must not be empty")
	}

	if len(component) == 0 {
		return errors.New("component must be provided and must not be empty")
	}

	if len(componentType) == 0 {
		return errors.New("component type must be provided and must not be empty")
	}

	if stackSection, ok = stacksMap[stack].(map[string]any); !ok {
		return fmt.Errorf("could not find the stack '%s'", stack)
	}

	if componentsSection, ok = stackSection["components"].(map[string]any); !ok {
		return fmt.Errorf("'components' section is missing in the stack manifest '%s'", stack)
	}

	if componentTypeSection, ok = componentsSection[componentType].(map[string]any); !ok {
		return fmt.Errorf("'components.%s' section is missing in the stack manifest '%s'", componentType, stack)
	}

	if componentSection, ok = componentTypeSection[component].(map[string]any); !ok {
		return fmt.Errorf("no config found for the component '%s' in the stack manifest '%s'", component, stack)
	}

	if componentVarsSection, ok = componentSection["vars"].(map[string]any); !ok {
		return fmt.Errorf("missing 'vars' section for the component '%s' in the stack manifest '%s'", component, stack)
	}

	if componentProvidersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
		componentProvidersSection = map[string]any{}
	}

	if componentHooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
		componentHooksSection = map[string]any{}
	}

	if componentBackendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
		componentBackendSection = nil
	}

	if componentBackendType, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
		componentBackendType = ""
	}

	if componentImportsSection, ok = stackSection["imports"].([]string); !ok {
		componentImportsSection = nil
	}

	if command, ok = componentSection[cfg.CommandSectionName].(string); !ok {
		command = ""
	}

	if componentEnvSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
		componentEnvSection = map[string]any{}
	}

	if componentAuthSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
		componentAuthSection = map[string]any{}
	}

	// Merge global auth config from atmosConfig if component doesn't have auth section.
	// This ensures profiles with auth config work even when auth.yaml is not explicitly imported.
	if len(componentAuthSection) == 0 && atmosConfig != nil {
		componentAuthSection = mergeGlobalAuthConfig(atmosConfig, componentSection)
	}

	if componentSettingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
		componentSettingsSection = map[string]any{}
	}

	if componentOverridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
		componentOverridesSection = map[string]any{}
	}

	if componentInheritanceChain, ok = componentSection["inheritance"].([]string); !ok {
		componentInheritanceChain = []string{}
	}

	// Process component metadata and find a base component (if any) and whether the component is real or abstract.
	componentMetadata, baseComponentName, componentIsAbstract, componentIsEnabled, componentIsLocked := ProcessComponentMetadata(component, componentSection)
	configAndStacksInfo.ComponentIsEnabled = componentIsEnabled
	configAndStacksInfo.ComponentIsLocked = componentIsLocked

	// Remove the ENV vars that are set to `null` in the `env` section.
	// Setting an ENV var to `null` in stack config has the effect of unsetting it.
	// This is because the exec.Command, which sets these ENV vars, is itself executed in a separate process started by the os.StartProcess function.
	componentEnvSectionFiltered := map[string]any{}

	for k, v := range componentEnvSection {
		if v != nil {
			componentEnvSectionFiltered[k] = v
		}
	}

	configAndStacksInfo.ComponentSection = componentSection
	configAndStacksInfo.ComponentVarsSection = componentVarsSection
	configAndStacksInfo.ComponentSettingsSection = componentSettingsSection
	configAndStacksInfo.ComponentOverridesSection = componentOverridesSection
	configAndStacksInfo.ComponentProvidersSection = componentProvidersSection
	configAndStacksInfo.StackSection = stackSection
	configAndStacksInfo.ComponentHooksSection = componentHooksSection
	configAndStacksInfo.ComponentEnvSection = componentEnvSectionFiltered
	configAndStacksInfo.ComponentAuthSection = componentAuthSection
	configAndStacksInfo.ComponentBackendSection = componentBackendSection
	configAndStacksInfo.ComponentBackendType = componentBackendType
	configAndStacksInfo.BaseComponentPath = baseComponentName
	configAndStacksInfo.ComponentInheritanceChain = componentInheritanceChain
	configAndStacksInfo.ComponentIsAbstract = componentIsAbstract
	configAndStacksInfo.ComponentMetadataSection = componentMetadata
	configAndStacksInfo.ComponentImportsSection = componentImportsSection

	if command != "" {
		configAndStacksInfo.Command = command
	}

	// Populate AuthContext from AuthManager if provided (from --identity flag).
	if authManager != nil {
		managerStackInfo := authManager.GetStackInfo()
		if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
			configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
		}
	}

	return nil
}

var (
	// FindStacksMapCache stores the results of FindStacksMap to avoid re-processing.
	// all YAML files multiple times within the same command execution.
	// Cache key: JSON-serialized atmosConfig key fields + ignoreMissingFiles flag.
	findStacksMapCache   map[string]*findStacksMapCacheEntry
	findStacksMapCacheMu sync.RWMutex
)

func init() {
	findStacksMapCache = make(map[string]*findStacksMapCacheEntry)
}

// findStacksMapCacheEntry stores the cached result of FindStacksMap.
type findStacksMapCacheEntry struct {
	stacksMap       map[string]any
	rawStackConfigs map[string]map[string]any
}

// getFindStacksMapCacheKey generates a content-aware cache key from atmosConfig and parameters.
// The cache key includes all paths, file lists, and modification times that affect stack processing,
// ensuring proper cache invalidation when configuration or file content changes.
func getFindStacksMapCacheKey(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) string {
	const cacheKeyDelimiter = "|"

	// Build a string containing all cache-relevant configuration.
	// Include all component directories that affect stack processing.
	var keyBuilder strings.Builder
	keyBuilder.WriteString(atmosConfig.StacksBaseAbsolutePath)
	keyBuilder.WriteString(cacheKeyDelimiter)
	keyBuilder.WriteString(atmosConfig.TerraformDirAbsolutePath)
	keyBuilder.WriteString(cacheKeyDelimiter)
	keyBuilder.WriteString(atmosConfig.HelmfileDirAbsolutePath)
	keyBuilder.WriteString(cacheKeyDelimiter)
	keyBuilder.WriteString(atmosConfig.PackerDirAbsolutePath)
	keyBuilder.WriteString(cacheKeyDelimiter)
	keyBuilder.WriteString(fmt.Sprintf("%v", ignoreMissingFiles))
	keyBuilder.WriteString(cacheKeyDelimiter)

	// Include the actual file paths and their modification times.
	// Sort the paths for consistent hashing.
	sortedPaths := make([]string, len(atmosConfig.StackConfigFilesAbsolutePaths))
	copy(sortedPaths, atmosConfig.StackConfigFilesAbsolutePaths)
	sort.Strings(sortedPaths)

	// Add all file paths and mtimes to the key.
	// This ensures cache invalidation when files are modified.
	for _, path := range sortedPaths {
		keyBuilder.WriteString(path)
		keyBuilder.WriteString(cacheKeyDelimiter)

		// Include file modification time and size for cache invalidation.
		// Use nanosecond precision to detect changes within the same second.
		// Include file size to detect content changes that preserve mtime.
		// If stat fails (file doesn't exist, permission denied, etc.),
		// use "missing:-1" sentinel to ensure consistent behavior.
		if info, err := os.Stat(path); err == nil {
			keyBuilder.WriteString(fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size()))
		} else {
			keyBuilder.WriteString("missing:-1")
		}
		keyBuilder.WriteString(cacheKeyDelimiter)
	}

	// Use SHA-256 hash to create a fixed-length cache key.
	// This prevents cache key explosion with large numbers of files.
	hash := sha256.Sum256([]byte(keyBuilder.String()))
	return hex.EncodeToString(hash[:])
}

// FindStacksMap processes stack config and returns a map of all stacks.
// Results are cached to avoid re-processing the same YAML files multiple times
// within the same command execution (e.g., when ValidateStacks is called before ExecuteDescribeStacks).
// ClearFindStacksMapCache clears the FindStacksMap cache.
func ClearFindStacksMapCache() {
	defer perf.Track(nil, "exec.ClearFindStacksMapCache")()

	log.Trace("ClearFindStacksMapCache called")
	findStacksMapCacheMu.Lock()
	findStacksMapCache = make(map[string]*findStacksMapCacheEntry)
	findStacksMapCacheMu.Unlock()
}

func FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	defer perf.Track(atmosConfig, "exec.FindStacksMap")()

	// Skip cache when provenance tracking is enabled, as we need to capture merge context and positions during processing.
	if !atmosConfig.TrackProvenance {
		// Generate cache key.
		cacheKey := getFindStacksMapCacheKey(atmosConfig, ignoreMissingFiles)

		// Check cache first.
		findStacksMapCacheMu.RLock()
		cached, found := findStacksMapCache[cacheKey]
		findStacksMapCacheMu.RUnlock()

		if found {
			return cached.stacksMap, cached.rawStackConfigs, nil
		}
	}

	// Cache miss - process stack config file(s).
	_, stacksMap, rawStackConfigs, err := ProcessYAMLConfigFiles(
		atmosConfig,
		atmosConfig.StacksBaseAbsolutePath,
		atmosConfig.TerraformDirAbsolutePath,
		atmosConfig.HelmfileDirAbsolutePath,
		atmosConfig.PackerDirAbsolutePath,
		atmosConfig.StackConfigFilesAbsolutePaths,
		false,
		true,
		ignoreMissingFiles,
	)
	if err != nil {
		return nil, nil, err
	}

	// Cache the result only when provenance tracking is disabled.
	if !atmosConfig.TrackProvenance {
		cacheKey := getFindStacksMapCacheKey(atmosConfig, ignoreMissingFiles)
		findStacksMapCacheMu.Lock()
		findStacksMapCache[cacheKey] = &findStacksMapCacheEntry{
			stacksMap:       stacksMap,
			rawStackConfigs: rawStackConfigs,
		}
		findStacksMapCacheMu.Unlock()
	}

	return stacksMap, rawStackConfigs, nil
}

// processStackContextPrefix processes the context prefix for a stack based on name template or pattern.
func processStackContextPrefix(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	stackName string,
) error {
	switch {
	case atmosConfig.Stacks.NameTemplate != "":
		tmpl, err := ProcessTmpl(atmosConfig, "name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return err
		}
		configAndStacksInfo.ContextPrefix = tmpl
	case atmosConfig.Stacks.NamePattern != "":
		// Process context.
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)

		var err error
		configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
			configAndStacksInfo.Context,
			GetStackNamePattern(atmosConfig),
			stackName,
		)
		if err != nil {
			return err
		}
	default:
		// No name_template or name_pattern configured - use filename as identity.
		// This enables zero-config stack naming for newcomers.
		configAndStacksInfo.ContextPrefix = stackName
	}

	configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
	configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
	return nil
}

// findComponentInStacks searches for a component across all stacks and returns matching stacks.
// Returns the count of found stacks, list of stack names, config info for the found component,
// and a map of filename->canonicalName for stacks where the canonical name differs from the filename.
func findComponentInStacks(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	stacksMap map[string]any,
	authManager auth.AuthManager,
) (int, []string, schema.ConfigAndStacksInfo, map[string]string) {
	foundStackCount := 0
	var foundStacks []string
	var foundConfigAndStacksInfo schema.ConfigAndStacksInfo
	// Track filename -> canonical name mappings for suggestion purposes.
	stackNameMappings := make(map[string]string)

	for stackName := range stacksMap {
		// Extract manifest name FIRST (before checking component) for suggestion purposes.
		// This allows us to suggest correct stack names even when the component isn't found.
		var stackManifestName string
		if stackSection, ok := stacksMap[stackName].(map[string]any); ok {
			if nameValue, ok := stackSection[cfg.NameSectionName].(string); ok {
				stackManifestName = nameValue
			}
		}

		// Track filename -> canonical name mapping for suggestion purposes.
		// We do this early so we can suggest correct names even if the component doesn't exist.
		if stackManifestName != "" && stackManifestName != stackName {
			stackNameMappings[stackName] = stackManifestName
		}

		// Check if we've found the component in the stack.
		err := ProcessComponentConfig(
			atmosConfig,
			configAndStacksInfo,
			stackName,
			stacksMap,
			configAndStacksInfo.ComponentType,
			configAndStacksInfo.ComponentFromArg,
			authManager,
		)
		if err != nil {
			continue
		}

		if err := processStackContextPrefix(atmosConfig, configAndStacksInfo, stackName); err != nil {
			continue
		}

		// Determine the canonical stack name using single-identity rule.
		// Each stack has exactly ONE valid identifier based on precedence:
		// 1. Explicit 'name' field in manifest (highest priority)
		// 2. Generated name from name_template or name_pattern (via ContextPrefix)
		// 3. Stack filename (only if nothing else is configured)
		//
		// See docs/prd/stack-name-identity.md for the full specification.
		var canonicalStackName string
		switch {
		case stackManifestName != "":
			// Priority 1: Explicit name from manifest.
			canonicalStackName = stackManifestName
		case configAndStacksInfo.ContextPrefix != "" && configAndStacksInfo.ContextPrefix != stackName:
			// Priority 2/3: Generated from name_template or name_pattern.
			// Only use if ContextPrefix differs from filename (indicates template/pattern was applied).
			canonicalStackName = configAndStacksInfo.ContextPrefix
		default:
			// Priority 4: Filename (when nothing else is configured).
			canonicalStackName = stackName
		}

		// Also track template/pattern-based canonical names (for stacks without explicit name).
		if canonicalStackName != stackName && stackManifestName == "" {
			stackNameMappings[stackName] = canonicalStackName
		}

		// Check if user's requested stack matches the canonical name.
		stackMatches := configAndStacksInfo.Stack == canonicalStackName

		if stackMatches {
			configAndStacksInfo.StackFile = stackName
			// Set StackManifestName if the stack has an explicit name.
			if stackManifestName != "" {
				configAndStacksInfo.StackManifestName = stackManifestName
			}
			foundConfigAndStacksInfo = *configAndStacksInfo
			foundStackCount++
			foundStacks = append(foundStacks, stackName)

			log.Debug(
				fmt.Sprintf("Found component '%s' in the stack '%s' in the stack manifest '%s'",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					stackName,
				))
		}
	}

	return foundStackCount, foundStacks, foundConfigAndStacksInfo, stackNameMappings
}

// ProcessStacks processes stack config.
func ProcessStacks(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	authManager auth.AuthManager,
) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStacks")()

	// Check if stack was provided.
	if checkStack && len(configAndStacksInfo.Stack) < 1 {
		return configAndStacksInfo, errUtils.ErrMissingStack
	}

	// Check if the component was provided.
	if len(configAndStacksInfo.ComponentFromArg) < 1 {
		message := fmt.Sprintf("`component` is required.\n\nUsage:\n\n`atmos %s <command> <component> <arguments_and_flags>`", configAndStacksInfo.ComponentType)
		return configAndStacksInfo, errors.New(message)
	}

	configAndStacksInfo.StackFromArg = configAndStacksInfo.Stack

	stacksMap, rawStackConfigs, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Print the stack config files.
	if atmosConfig.Logs.Level == u.LogLevelTrace {
		var msg string
		if atmosConfig.StackType == "Directory" {
			msg = "\nFound stack manifest:"
		} else {
			msg = "\nFound stack manifests:"
		}
		log.Debug(msg)
		err = u.PrintAsYAMLToFileDescriptor(atmosConfig, atmosConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return configAndStacksInfo, err
		}
	}

	// Check and process stacks.
	if atmosConfig.StackType == "Directory" {
		err = ProcessComponentConfig(
			atmosConfig,
			&configAndStacksInfo,
			configAndStacksInfo.Stack,
			stacksMap,
			configAndStacksInfo.ComponentType,
			configAndStacksInfo.ComponentFromArg,
			authManager,
		)
		if err != nil {
			return configAndStacksInfo, err
		}

		configAndStacksInfo.StackFile = configAndStacksInfo.Stack

		// Process context.
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
		configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath

		configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
			configAndStacksInfo.Context,
			GetStackNamePattern(atmosConfig),
			configAndStacksInfo.Stack,
		)
		if err != nil {
			return configAndStacksInfo, err
		}
	} else {
		foundStackCount, foundStacks, foundConfigAndStacksInfo, stackNameMappings := findComponentInStacks(
			atmosConfig,
			&configAndStacksInfo,
			stacksMap,
			authManager,
		)

		if foundStackCount == 0 && !checkStack {
			// Allow proceeding without error if checkStack is false (e.g., for operations that don't require a stack).
			return configAndStacksInfo, nil
		}

		// Only attempt path resolution fallback if the component argument looks like a file path.
		// This prevents treating plain component names (like "vpc") as relative paths from CWD.
		//
		// Note: Component names in stack configs can contain forward slashes (like "infra/vpc"),
		// but these are namespace prefixes, not file paths. The key distinction is:
		// - "infra/vpc" as a component name → already found in first loop, no fallback needed
		// - "components/terraform/vpc" as a path → not found in first loop, fallback needed
		//
		// Path indicators (trigger fallback):
		// - Forward slash: "components/terraform/vpc"
		// - Backslash (Windows): "components\terraform\vpc"
		// - Dot paths: ".", "..", "./vpc"
		//
		// Examples that should NOT trigger fallback: "vpc", "top-level-component1", "infra/vpc" (if defined in stack)
		pathArg := configAndStacksInfo.ComponentFromArg
		hasForwardSlash := strings.Contains(pathArg, "/")
		hasPlatformSep := filepath.Separator != '/' && strings.ContainsRune(pathArg, filepath.Separator)
		// Check for dot paths with both forward slash and platform separator.
		isDotPath := pathArg == "." || pathArg == ".." ||
			strings.HasPrefix(pathArg, "./") || strings.HasPrefix(pathArg, "../") ||
			strings.HasPrefix(pathArg, "."+string(filepath.Separator)) ||
			strings.HasPrefix(pathArg, ".."+string(filepath.Separator))
		shouldAttemptPathResolution := foundStackCount == 0 && (hasForwardSlash || hasPlatformSep || isDotPath)

		if shouldAttemptPathResolution {
			// Component not found - try fallback to path resolution.
			// If the component argument looks like it could be a path (e.g., "components/terraform/vpc"),
			// try resolving it as a filesystem path and retry with the resolved component name.
			log.Debug("Component not found by name, attempting path resolution fallback",
				"component", configAndStacksInfo.ComponentFromArg,
				"stack", configAndStacksInfo.Stack,
			)

			resolvedComponent, pathErr := ResolveComponentFromPathWithoutValidation(
				atmosConfig,
				configAndStacksInfo.ComponentFromArg,
				configAndStacksInfo.ComponentType,
			)

			if pathErr == nil {
				// Path resolution succeeded - retry with resolved component name.
				log.Debug("Path resolution succeeded, retrying with resolved component",
					"original", configAndStacksInfo.ComponentFromArg,
					"resolved", resolvedComponent,
				)

				// Update ComponentFromArg with resolved name and retry the loop.
				configAndStacksInfo.ComponentFromArg = resolvedComponent

				foundStackCount, foundStacks, foundConfigAndStacksInfo, stackNameMappings = findComponentInStacks(
					atmosConfig,
					&configAndStacksInfo,
					stacksMap,
					authManager,
				)
			} else if errors.Is(pathErr, errUtils.ErrPathNotInComponentDir) {
				// Path resolution failed because path is not in component directories.
				// Return the detailed path error instead of generic "component not found".
				return configAndStacksInfo, pathErr
			}
		}

		// If still not found after path resolution attempt (or if path resolution was skipped), return error.
		if foundStackCount == 0 {
			// Check if the user provided a filename that has a different canonical name.
			// This helps users who try to use the filename when the stack has an explicit name.
			if canonicalName, found := stackNameMappings[configAndStacksInfo.Stack]; found {
				return configAndStacksInfo,
					errUtils.Build(errUtils.ErrInvalidStack).
						WithExplanation(fmt.Sprintf("Stack `%s` not found.", configAndStacksInfo.Stack)).
						WithHint(fmt.Sprintf("Did you mean `%s`?", canonicalName)).
						Err()
			}

			cliConfigYaml := ""

			if atmosConfig.Logs.Level == u.LogLevelTrace {
				y, _ := u.ConvertToYAML(atmosConfig)
				cliConfigYaml = fmt.Sprintf("\n\n\nCLI config: %v\n", y)
			}

			return configAndStacksInfo,
				fmt.Errorf("%w: Could not find the component `%s` in the stack `%s`.\n"+
					"Check that all the context variables are correctly defined in the stack manifests.\n"+
					"Are the component and stack names correct? Did you forget an import?%v",
					errUtils.ErrInvalidComponent,
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					cliConfigYaml)
		} else if foundStackCount > 1 {
			err = fmt.Errorf("%w: Found duplicate config for the component `%s` in the stack `%s` in the manifests: %v\n"+
				"Check that all the context variables are correctly defined in the manifests and not duplicated\n"+
				"Check that all imports are valid",
				errUtils.ErrInvalidComponent,
				configAndStacksInfo.ComponentFromArg,
				configAndStacksInfo.Stack,
				strings.Join(foundStacks, ", "),
			)
			errUtils.CheckErrorPrintAndExit(err, "", "")
		} else {
			configAndStacksInfo = foundConfigAndStacksInfo
		}
	}

	if configAndStacksInfo.ComponentSection == nil {
		configAndStacksInfo.ComponentSection = make(map[string]any)
	}

	// Add imports.
	configAndStacksInfo.ComponentSection["imports"] = configAndStacksInfo.ComponentImportsSection

	// Add Atmos component and stack.
	configAndStacksInfo.ComponentSection["atmos_component"] = configAndStacksInfo.ComponentFromArg
	configAndStacksInfo.ComponentSection["atmos_stack"] = configAndStacksInfo.StackFromArg
	configAndStacksInfo.ComponentSection["stack"] = configAndStacksInfo.StackFromArg
	configAndStacksInfo.ComponentSection["atmos_stack_file"] = configAndStacksInfo.StackFile
	configAndStacksInfo.ComponentSection["atmos_manifest"] = configAndStacksInfo.StackFile

	// If the command-line component does not inherit anything, then the Terraform/Helmfile component is the same as the provided one.
	if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
		configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = configAndStacksInfo.ComponentFromArg
	}

	// `sources` (stack config files where the variables and other settings are defined).
	sources, err := ProcessConfigSources(configAndStacksInfo, rawStackConfigs)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.ComponentSection["sources"] = sources

	// Component dependencies.
	componentDeps, componentDepsAll, err := FindComponentDependencies(configAndStacksInfo.StackFile, sources)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.ComponentSection["deps"] = componentDeps
	configAndStacksInfo.ComponentSection["deps_all"] = componentDepsAll

	// Terraform workspace.
	workspace, err := BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.TerraformWorkspace = workspace
	configAndStacksInfo.ComponentSection["workspace"] = workspace

	// Process `Go` templates in Atmos manifest sections.
	if processTemplates {
		componentSectionStr, err := u.ConvertToYAML(configAndStacksInfo.ComponentSection)
		if err != nil {
			return configAndStacksInfo, err
		}

		var settingsSectionStruct schema.Settings

		err = mapstructure.Decode(configAndStacksInfo.ComponentSettingsSection, &settingsSectionStruct)
		if err != nil {
			return configAndStacksInfo, err
		}

		componentSectionProcessed, err := ProcessTmplWithDatasources(
			atmosConfig,
			&configAndStacksInfo,
			settingsSectionStruct,
			"templates-all-atmos-sections",
			componentSectionStr,
			configAndStacksInfo.ComponentSection,
			true,
		)
		if err != nil {
			// If any error returned from the template processing, log it and exit.
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}

		componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
		if err != nil {
			if !atmosConfig.Templates.Settings.Enabled {
				if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
					errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
						"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
					err = errors.Join(err, errors.New(errorMessage))
				}
			}
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}

		configAndStacksInfo.ComponentSection = componentSectionConverted
	}

	// Process YAML functions in Atmos manifest sections.
	if processYamlFunctions {
		componentSectionConverted, err := ProcessCustomYamlTags(atmosConfig, configAndStacksInfo.ComponentSection, configAndStacksInfo.Stack, skip, &configAndStacksInfo)
		if err != nil {
			return configAndStacksInfo, err
		}

		configAndStacksInfo.ComponentSection = componentSectionConverted
	}

	if processTemplates || processYamlFunctions {
		postProcessTemplatesAndYamlFunctions(&configAndStacksInfo)
	}

	// Spacelift stack.
	spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}
	if spaceliftStackName != "" {
		configAndStacksInfo.ComponentSection["spacelift_stack"] = spaceliftStackName
	}

	// Atlantis project.
	atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}
	if atlantisProjectName != "" {
		configAndStacksInfo.ComponentSection["atlantis_project"] = atlantisProjectName
	}

	// Process the ENV variables from the `env` section.
	configAndStacksInfo.ComponentEnvList = env.ConvertEnvVars(configAndStacksInfo.ComponentEnvSection)

	// Process component metadata.
	_, baseComponentName, _, componentIsEnabled, componentIsLocked := ProcessComponentMetadata(configAndStacksInfo.ComponentFromArg, configAndStacksInfo.ComponentSection)
	configAndStacksInfo.BaseComponentPath = baseComponentName
	configAndStacksInfo.ComponentIsEnabled = componentIsEnabled
	configAndStacksInfo.ComponentIsLocked = componentIsLocked

	// Process component path and name.
	configAndStacksInfo.ComponentFolderPrefix = ""
	componentPathParts := strings.Split(configAndStacksInfo.ComponentFromArg, "/")
	componentPathPartsLength := len(componentPathParts)
	if componentPathPartsLength > 1 {
		componentFromArgPartsWithoutLast := componentPathParts[:componentPathPartsLength-1]
		configAndStacksInfo.ComponentFolderPrefix = strings.Join(componentFromArgPartsWithoutLast, "/")
		configAndStacksInfo.Component = componentPathParts[componentPathPartsLength-1]
	} else {
		configAndStacksInfo.Component = configAndStacksInfo.ComponentFromArg
	}
	configAndStacksInfo.ComponentFolderPrefixReplaced = strings.Replace(configAndStacksInfo.ComponentFolderPrefix, "/", "-", -1)

	// Process base component path and name.
	if len(configAndStacksInfo.BaseComponentPath) > 0 {
		baseComponentPathParts := strings.Split(configAndStacksInfo.BaseComponentPath, "/")
		baseComponentPathPartsLength := len(baseComponentPathParts)
		if baseComponentPathPartsLength > 1 {
			baseComponentPartsWithoutLast := baseComponentPathParts[:baseComponentPathPartsLength-1]
			configAndStacksInfo.ComponentFolderPrefix = strings.Join(baseComponentPartsWithoutLast, "/")
			configAndStacksInfo.BaseComponent = baseComponentPathParts[baseComponentPathPartsLength-1]
		} else {
			configAndStacksInfo.ComponentFolderPrefix = ""
			configAndStacksInfo.BaseComponent = configAndStacksInfo.BaseComponentPath
		}
		configAndStacksInfo.ComponentFolderPrefixReplaced = strings.Replace(configAndStacksInfo.ComponentFolderPrefix, "/", "-", -1)
	}

	// Get the final component.
	if len(configAndStacksInfo.BaseComponent) > 0 {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.BaseComponent
	} else {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.Component
	}

	// Add component info, including Terraform config.
	componentInfo := map[string]any{}
	componentInfo["component_type"] = configAndStacksInfo.ComponentType

	switch configAndStacksInfo.ComponentType {
	case cfg.TerraformComponentType:
		componentPath := constructTerraformComponentWorkingDir(atmosConfig, &configAndStacksInfo)
		componentInfo[cfg.ComponentPathSectionName] = componentPath
		terraformConfiguration, diags := tfconfig.LoadModule(componentPath)
		if !diags.HasErrors() {
			componentInfo[terraformConfigKey] = terraformConfiguration
		} else {
			diagErr := diags.Err()

			// Handle edge case where Err() returns nil despite HasErrors() being true.
			if diagErr == nil {
				componentInfo[terraformConfigKey] = nil
			} else {
				// Try structured error detection first (most robust).
				isNotExist := errors.Is(diagErr, os.ErrNotExist) || errors.Is(diagErr, fs.ErrNotExist)

				// Fallback to error message inspection for cases where tfconfig doesn't wrap errors properly.
				// This handles missing subdirectory modules (e.g., ./modules/security-group referenced in main.tf
				// but the directory doesn't exist). Such missing paths are valid in stack processing—components
				// or their modules may be deleted or not yet created when tracking changes over time.
				errMsg := diagErr.Error()
				isNotExistString := strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "Failed to read directory")

				if !isNotExist && !isNotExistString {
					// Check if this is an OpenTofu-specific feature that terraform-config-inspect doesn't support.
					// Respect component-level command overrides for OpenTofu detection.
					// Clone the config and apply the component override if present.
					effectiveConfig := *atmosConfig
					if configAndStacksInfo.Command != "" {
						effectiveConfig.Components.Terraform.Command = configAndStacksInfo.Command
					}

					// For known OpenTofu features, skip validation. Otherwise, return the error.
					if !IsOpenTofu(&effectiveConfig) || !isKnownOpenTofuFeature(diagErr) {
						// For other errors (syntax errors, permission issues, etc.), return error.
						// Use ErrorBuilder to provide helpful context about the HCL parsing failure.
						// This fixes https://github.com/cloudposse/atmos/issues/1864 by showing a clear error
						// instead of the misleading "component not found" message.

						// Extract file and line information from diagnostics if available.
						var fileLocation string
						for _, diag := range diags {
							if diag.Pos != nil {
								fileLocation = fmt.Sprintf("%s:%d", diag.Pos.Filename, diag.Pos.Line)
								break // Only show the first location.
							}
						}

						// Build explanation with file location if available.
						var explanation string
						if fileLocation != "" {
							explanation = fmt.Sprintf("The Terraform component '%s' contains invalid HCL code at %s.",
								configAndStacksInfo.ComponentFromArg, fileLocation)
						} else {
							explanation = fmt.Sprintf("The Terraform component '%s' contains invalid HCL code.",
								configAndStacksInfo.ComponentFromArg)
						}

						err := errUtils.Build(errUtils.ErrFailedToLoadTerraformComponent).
							WithCause(diagErr).
							WithExplanation(explanation).
							WithHintf("Run 'atmos terraform validate' to see more details:\n```\natmos terraform validate %s -s %s\n```",
								configAndStacksInfo.ComponentFromArg, configAndStacksInfo.Stack).
							Err()

						return configAndStacksInfo, err
					}

					// Skip validation for known OpenTofu-specific features.
					log.Debug("Skipping terraform-config-inspect validation for OpenTofu-specific feature: " + errMsg)
					componentInfo[terraformConfigKey] = nil
					componentInfo["validation_skipped_opentofu"] = true
				} else {
					componentInfo[terraformConfigKey] = nil
				}
			}
		}
	case cfg.HelmfileComponentType:
		componentInfo[cfg.ComponentPathSectionName] = constructHelmfileComponentWorkingDir(atmosConfig, &configAndStacksInfo)
	case cfg.PackerComponentType:
		componentInfo[cfg.ComponentPathSectionName] = constructPackerComponentWorkingDir(atmosConfig, &configAndStacksInfo)
	}

	configAndStacksInfo.ComponentSection["component_info"] = componentInfo

	// Add command-line arguments and vars to the component section.
	// It will allow using them when validating with OPA policies or JSON Schema.
	args := append(configAndStacksInfo.CliArgs, configAndStacksInfo.AdditionalArgsAndFlags...)

	var filteredArgs []string
	for _, item := range args {
		if item != "" {
			filteredArgs = append(filteredArgs, item)
		}
	}

	configAndStacksInfo.ComponentSection[cfg.CliArgsSectionName] = filteredArgs

	cliVars, err := getCliVars(configAndStacksInfo.AdditionalArgsAndFlags)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.ComponentSection[cfg.TerraformCliVarsSectionName] = cliVars

	// Add TF_CLI_ARGS arguments and variables to the component section.
	tfEnvCliArgs := GetTerraformEnvCliArgs()
	if len(tfEnvCliArgs) > 0 {
		configAndStacksInfo.ComponentSection[cfg.TerraformCliArgsEnvSectionName] = tfEnvCliArgs
	}

	tfEnvCliVars, err := GetTerraformEnvCliVars()
	if err != nil {
		return configAndStacksInfo, err
	}
	if len(tfEnvCliVars) > 0 {
		configAndStacksInfo.ComponentSection[cfg.TerraformCliVarsEnvSectionName] = tfEnvCliVars
	}

	// Add Atmos CLI config.
	atmosCliConfig := map[string]any{}
	atmosCliConfig["base_path"] = atmosConfig.BasePath
	atmosCliConfig["components"] = atmosConfig.Components
	atmosCliConfig["stacks"] = atmosConfig.Stacks
	atmosCliConfig["workflows"] = atmosConfig.Workflows
	configAndStacksInfo.ComponentSection["atmos_cli_config"] = atmosCliConfig

	return configAndStacksInfo, nil
}

// generateComponentBackendConfig generates backend config for components.
func generateComponentBackendConfig(backendType string, backendConfig map[string]any, terraformWorkspace string, _ *schema.AuthContext) (map[string]any, error) {
	// Validate that backendType is not empty to avoid generating invalid backend config.
	// An empty backendType would result in invalid JSON like: {"terraform": {"backend": {"": {}}}}.
	if backendType == "" {
		return nil, errUtils.ErrBackendTypeRequired
	}

	// Generate backend config file for Terraform Cloud.
	// https://developer.hashicorp.com/terraform/cli/cloud/settings
	if backendType == "cloud" {
		backendConfigFinal := backendConfig

		if terraformWorkspace != "" {
			// Process template tokens in the backend config.
			backendConfigStr, err := u.ConvertToYAML(backendConfig)
			if err != nil {
				return nil, err
			}

			ctx := schema.Context{
				TerraformWorkspace: terraformWorkspace,
			}

			backendConfigStrReplaced := cfg.ReplaceContextTokens(ctx, backendConfigStr)

			backendConfigFinal, err = u.UnmarshalYAML[schema.AtmosSectionMapType](backendConfigStrReplaced)
			if err != nil {
				return nil, err
			}
		}

		return map[string]any{
			"terraform": map[string]any{
				"cloud": backendConfigFinal,
			},
		}, nil
	}

	// Generate backend config file for all other Terraform backends.
	return map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				backendType: backendConfig,
			},
		},
	}, nil
}

// generateComponentProviderOverrides generates provider overrides for components.
func generateComponentProviderOverrides(providerOverrides map[string]any, _ *schema.AuthContext) map[string]any {
	return map[string]any{
		"provider": providerOverrides,
	}
}

// FindComponentDependencies finds all imports that the component depends on, and all imports that the component has any sections defined in.
func FindComponentDependencies(currentStack string, sources schema.ConfigSources) ([]string, []string, error) {
	defer perf.Track(nil, "exec.FindComponentDependencies")()

	var deps []string
	var depsAll []string

	for _, source := range sources {
		for _, v := range source {
			for i, dep := range v.StackDependencies {
				if dep.StackFile != "" {
					depsAll = append(depsAll, dep.StackFile)
					if i == 0 {
						deps = append(deps, dep.StackFile)
					}
				}
			}
		}
	}

	depsAll = append(depsAll, currentStack)
	unique := u.UniqueStrings(deps)
	uniqueAll := u.UniqueStrings(depsAll)
	sort.Strings(unique)
	sort.Strings(uniqueAll)
	return unique, uniqueAll, nil
}

// postProcessTemplatesAndYamlFunctions restores Atmos sections after processing `Go` templates and custom YAML functions/tags.
func postProcessTemplatesAndYamlFunctions(configAndStacksInfo *schema.ConfigAndStacksInfo) {
	if i, ok := configAndStacksInfo.ComponentSection[cfg.ProvidersSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentProvidersSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.AuthSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentAuthSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.VarsSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentVarsSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.SettingsSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentSettingsSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.EnvSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentEnvSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.OverridesSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentOverridesSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.MetadataSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentMetadataSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.BackendSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentBackendSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.BackendTypeSectionName].(string); ok {
		configAndStacksInfo.ComponentBackendType = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); ok {
		configAndStacksInfo.Component = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.CommandSectionName].(string); ok {
		configAndStacksInfo.Command = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.WorkspaceSectionName].(string); ok {
		configAndStacksInfo.TerraformWorkspace = i
	}
}
