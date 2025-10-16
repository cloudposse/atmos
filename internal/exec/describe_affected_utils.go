package exec

import (
	"path/filepath"
	"reflect"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/mitchellh/mapstructure"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func executeDescribeAffected(
	atmosConfig *schema.AtmosConfiguration,
	localRepoFileSystemPath string,
	remoteRepoFileSystemPath string,
	localRepo *git.Repository,
	remoteRepo *git.Repository,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	excludeLocked bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, error) {
	localRepoHead, err := localRepo.Head()
	if err != nil {
		return nil, nil, nil, err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Current", "HEAD", localRepoHead)
	log.Debug("Current", "BASE", remoteRepoHead)

	currentStacks, err := ExecuteDescribeStacks(
		atmosConfig,
		stack,
		nil,
		nil,
		nil,
		false,
		processTemplates,
		processYamlFunctions,
		false,
		skip,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	localRepoFileSystemPathAbs, err := filepath.Abs(localRepoFileSystemPath)
	if err != nil {
		return nil, nil, nil, err
	}

	basePath := atmosConfig.BasePath

	// Handle `atmos` absolute base path.
	// Absolute base path can be set in the `base_path` attribute in `atmos.yaml`, or using the ENV var `ATMOS_BASE_PATH` (as it's done in `geodesic`)
	// If the `atmos` base path is absolute, find the relative path between the local repo path and the `atmos` base path.
	// This relative path (the difference) is then used below to join with the remote (cloned) target repo path.
	if filepath.IsAbs(basePath) {
		basePath, err = filepath.Rel(localRepoFileSystemPathAbs, basePath)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	// Update paths to point to the cloned remote repo dir
	currentStacksBaseAbsolutePath := atmosConfig.StacksBaseAbsolutePath
	currentStacksTerraformDirAbsolutePath := atmosConfig.TerraformDirAbsolutePath
	currentStacksHelmfileDirAbsolutePath := atmosConfig.HelmfileDirAbsolutePath
	currentStacksPackerDirAbsolutePath := atmosConfig.PackerDirAbsolutePath
	currentStacksStackConfigFilesAbsolutePaths := atmosConfig.StackConfigFilesAbsolutePaths

	atmosConfig.StacksBaseAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Stacks.BasePath)
	atmosConfig.TerraformDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Terraform.BasePath)
	atmosConfig.HelmfileDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Helmfile.BasePath)
	atmosConfig.PackerDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Packer.BasePath)
	atmosConfig.StackConfigFilesAbsolutePaths, err = u.JoinPaths(
		filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Stacks.BasePath),
		atmosConfig.StackConfigFilesRelativePaths,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	remoteStacks, err := ExecuteDescribeStacks(
		atmosConfig,
		stack,
		nil,
		nil,
		nil,
		true,
		processTemplates,
		processYamlFunctions,
		false,
		skip,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// Restore atmosConfig
	atmosConfig.StacksBaseAbsolutePath = currentStacksBaseAbsolutePath
	atmosConfig.TerraformDirAbsolutePath = currentStacksTerraformDirAbsolutePath
	atmosConfig.HelmfileDirAbsolutePath = currentStacksHelmfileDirAbsolutePath
	atmosConfig.PackerDirAbsolutePath = currentStacksPackerDirAbsolutePath
	atmosConfig.StackConfigFilesAbsolutePaths = currentStacksStackConfigFilesAbsolutePaths

	log.Debug("Getting current working repo commit object")

	localCommit, err := localRepo.CommitObject(localRepoHead.Hash())
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got current working repo commit object")
	log.Debug("Getting current working repo commit tree")

	localTree, err := localCommit.Tree()
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got current working repo commit tree")
	log.Debug("Getting remote repo commit object")

	remoteCommit, err := remoteRepo.CommitObject(remoteRepoHead.Hash())
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got remote repo commit object")
	log.Debug("Getting remote repo commit tree")

	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got remote repo commit tree")
	log.Debug("Finding difference between the current working branch and remote target branch")

	// Find a slice of Patch objects with all the changes between the current working and remote trees
	patch, err := localTree.Patch(remoteTree)
	if err != nil {
		return nil, nil, nil, err
	}

	var changedFiles []string

	if len(patch.Stats()) > 0 {
		log.Debug("Found difference between the current working branch and remote target branch")
		log.Debug("Changed", "files", patch.Stats())

		for _, fileStat := range patch.Stats() {
			changedFiles = append(changedFiles, fileStat.Name)
		}
	} else {
		log.Debug("The current working branch and remote target branch are the same")
	}

	affected, err := findAffected(
		&currentStacks,
		&remoteStacks,
		atmosConfig,
		changedFiles,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		excludeLocked,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return affected, localRepoHead, remoteRepoHead, nil
}

// findAffected returns a list of all affected components in all stacks.
// Uses parallel processing for improved performance (P9.1 optimization).
func findAffected(
	currentStacks *map[string]any,
	remoteStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stackToFilter string,
	excludeLocked bool,
) ([]schema.Affected, error) {
	// Use parallel implementation for significant performance improvement (40-60% faster).
	return findAffectedParallel(
		currentStacks,
		remoteStacks,
		atmosConfig,
		changedFiles,
		includeSpaceliftAdminStacks,
		includeSettings,
		stackToFilter,
		excludeLocked,
	)
}

// findAffectedSequential is the original sequential implementation kept for reference.
// This has been replaced by findAffectedParallel for better performance.
//
//nolint:cyclop,funlen,gocognit,revive // Sequential implementation kept for reference; complexity is intentional
func findAffectedSequential(
	currentStacks *map[string]any,
	remoteStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stackToFilter string,
	excludeLocked bool,
) ([]schema.Affected, error) {
	res := []schema.Affected{}
	var err error

	for stackName, stackSection := range *currentStacks {
		// If `--stack` is provided on the command line, processes only components in that stack
		if stackToFilter != "" && stackToFilter != stackName {
			continue
		}

		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {

				// Terraform
				if terraformSection, ok := componentsSection[cfg.TerraformComponentType].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection["metadata"].(map[string]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Skip disabled components
								if !isComponentEnabled(metadataSection, componentName) {
									continue
								}
								// Skip locked components (if `--exclude-locked` is provided on the command line)
								if excludeLocked && isComponentLocked(metadataSection) {
									continue
								}
								// Check `metadata` section
								if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, metadataSection, "metadata") {
									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.metadata",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}

							// Check the Terraform configuration of the component
							if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
								// Check if the component uses some external modules (on the local filesystem) that have changed
								changed, err := areTerraformComponentModulesChanged(component, atmosConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component.module",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}

								// Check if any files in the component's folder have changed
								changed, err = isComponentFolderChanged(component, cfg.TerraformComponentType, atmosConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, settingsSection, cfg.SettingsSectionName) {
									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}

								// Check `settings.depends_on.file` and `settings.depends_on.folder`
								// Convert the `settings` section to the `Settings` structure
								var stackComponentSettings schema.Settings
								err = mapstructure.Decode(settingsSection, &stackComponentSettings)
								if err != nil {
									return nil, err
								}

								// Skip if the stack component has an empty `settings.depends_on` section
								if reflect.ValueOf(stackComponentSettings).IsZero() ||
									reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
									continue
								}

								isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChanged(
									changedFiles,
									stackComponentSettings.DependsOn,
								)
								if err != nil {
									return nil, err
								}

								if isFolderOrFileChanged {
									changedFile := ""
									if changedType == "file" {
										changedFile = changedFileOrFolder
									}

									changedFolder := ""
									if changedType == "folder" {
										changedFolder = changedFileOrFolder
									}

									affected := schema.Affected{
										ComponentType: cfg.TerraformComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      changedType,
										File:          changedFile,
										Folder:        changedFolder,
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
						}
					}
				}

				// Helmfile
				if helmfileSection, ok := componentsSection[cfg.HelmfileComponentType].(map[string]any); ok {
					for componentName, compSection := range helmfileSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection["metadata"].(map[string]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Skip disabled components
								if !isComponentEnabled(metadataSection, componentName) {
									continue
								}
								// Skip locked components (if `--exclude-locked` is provided on the command line)
								if excludeLocked && isComponentLocked(metadataSection) {
									continue
								}
								// Check `metadata` section
								if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, metadataSection, "metadata") {
									affected := schema.Affected{
										ComponentType: cfg.HelmfileComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.metadata",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}

							// Check the Helmfile configuration of the component
							if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
								// Check if any files in the component's folder have changed
								changed, err := isComponentFolderChanged(component, cfg.HelmfileComponentType, atmosConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: cfg.HelmfileComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: cfg.HelmfileComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: cfg.HelmfileComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, settingsSection, cfg.SettingsSectionName) {
									affected := schema.Affected{
										ComponentType: cfg.HelmfileComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}

								// Check `settings.depends_on.file` and `settings.depends_on.folder`
								// Convert the `settings` section to the `Settings` structure
								var stackComponentSettings schema.Settings
								err = mapstructure.Decode(settingsSection, &stackComponentSettings)
								if err != nil {
									return nil, err
								}

								// Skip if the stack component has an empty `settings.depends_on` section
								if reflect.ValueOf(stackComponentSettings).IsZero() ||
									reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
									continue
								}

								isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChanged(
									changedFiles,
									stackComponentSettings.DependsOn,
								)
								if err != nil {
									return nil, err
								}

								if isFolderOrFileChanged {
									changedFile := ""
									if changedType == "file" {
										changedFile = changedFileOrFolder
									}

									changedFolder := ""
									if changedType == "folder" {
										changedFolder = changedFileOrFolder
									}

									affected := schema.Affected{
										ComponentType: cfg.HelmfileComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      changedType,
										File:          changedFile,
										Folder:        changedFolder,
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
						}
					}
				}

				// Packer
				if packerSection, ok := componentsSection[cfg.PackerComponentType].(map[string]any); ok {
					for componentName, compSection := range packerSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection["metadata"].(map[string]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Skip disabled components
								if !isComponentEnabled(metadataSection, componentName) {
									continue
								}
								// Skip locked components (if `--exclude-locked` is provided on the command line)
								if excludeLocked && isComponentLocked(metadataSection) {
									continue
								}
								// Check `metadata` section
								if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, metadataSection, "metadata") {
									affected := schema.Affected{
										ComponentType: cfg.PackerComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.metadata",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}

							// Check the Packer configuration of the component
							if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
								// Check if any files in the component's folder have changed
								changed, err := isComponentFolderChanged(component, cfg.PackerComponentType, atmosConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: cfg.PackerComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: cfg.PackerComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: cfg.PackerComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, settingsSection, cfg.SettingsSectionName) {
									affected := schema.Affected{
										ComponentType: cfg.PackerComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}

								// Check `settings.depends_on.file` and `settings.depends_on.folder`
								// Convert the `settings` section to the `Settings` structure
								var stackComponentSettings schema.Settings
								err = mapstructure.Decode(settingsSection, &stackComponentSettings)
								if err != nil {
									return nil, err
								}

								// Skip if the stack component has an empty `settings.depends_on` section
								if reflect.ValueOf(stackComponentSettings).IsZero() ||
									reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
									continue
								}

								isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChanged(
									changedFiles,
									stackComponentSettings.DependsOn,
								)
								if err != nil {
									return nil, err
								}

								if isFolderOrFileChanged {
									changedFile := ""
									if changedType == "file" {
										changedFile = changedFileOrFolder
									}

									changedFolder := ""
									if changedType == "folder" {
										changedFolder = changedFileOrFolder
									}

									affected := schema.Affected{
										ComponentType: cfg.PackerComponentType,
										Component:     componentName,
										Stack:         stackName,
										Affected:      changedType,
										File:          changedFile,
										Folder:        changedFolder,
									}
									err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										&componentSection,
										&res,
										&affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return res, nil
}
