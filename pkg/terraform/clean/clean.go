// Package clean provides functionality to clean up Terraform state and artifacts.
package clean

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// Flags for clean options.
const (
	ForceFlag                 = "--force"
	EverythingFlag            = "--everything"
	SkipTerraformLockFileFlag = "--skip-lock-file"
)

// File and directory patterns for Terraform artifacts.
const (
	// TerraformDir is the standard Terraform working directory.
	TerraformDir = ".terraform"
	// TerraformLockFile is the dependency lock file.
	TerraformLockFile = ".terraform.lock.hcl"
	// TerraformVarFileGlob is the glob pattern for generated variable files.
	TerraformVarFileGlob = "*.tfvars.json"
	// TerraformStateDir is the directory for local state files.
	TerraformStateDir = "terraform.tfstate.d"
	// BackendConfigFile is the auto-generated backend configuration file.
	BackendConfigFile = "backend.tf.json"
)

// StackProcessor defines the interface for stack operations needed by clean.
type StackProcessor interface {
	// ProcessStacks processes stacks and returns component configuration.
	ProcessStacks(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error)

	// ExecuteDescribeStacks describes stacks for the clean operation.
	ExecuteDescribeStacks(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
	) (map[string]any, error)

	// GetGenerateFilenamesForComponent returns auto-generated filenames for a component.
	GetGenerateFilenamesForComponent(componentSection map[string]any) []string

	// CollectComponentsDirectoryObjects collects directory objects for components.
	CollectComponentsDirectoryObjects(basePath string, componentPaths []string, patterns []string) ([]Directory, error)

	// ConstructTerraformComponentVarfileName constructs the varfile name.
	ConstructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string

	// ConstructTerraformComponentPlanfileName constructs the planfile name.
	ConstructTerraformComponentPlanfileName(info *schema.ConfigAndStacksInfo) string

	// GetAllStacksComponentsPaths returns all component paths from stacks.
	GetAllStacksComponentsPaths(stacksMap map[string]any) []string
}

// Service provides clean operations for Terraform components.
type Service struct {
	processor StackProcessor
}

// NewService creates a new clean service with the given stack processor.
func NewService(processor StackProcessor) *Service {
	defer perf.Track(nil, "clean.NewService")()

	return &Service{
		processor: processor,
	}
}

// Options holds options for the clean operation.
type Options struct {
	Component    string
	Stack        string
	Force        bool
	Everything   bool
	SkipLockFile bool
	DryRun       bool
	Cache        bool
}

// Execute cleans up Terraform state and artifacts for a component.
func (s *Service) Execute(opts *Options, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "clean.Execute")()

	if opts == nil {
		return errUtils.ErrNilParam
	}

	log.Debug("Execute called",
		"component", opts.Component,
		"stack", opts.Stack,
		"force", opts.Force,
		"everything", opts.Everything,
		"skipLockFile", opts.SkipLockFile,
		"dryRun", opts.DryRun,
		"cache", opts.Cache,
	)

	// Handle --cache flag: clean the plugin cache directory.
	if opts.Cache {
		return s.cleanPluginCache(opts, atmosConfig)
	}

	// Build ConfigAndStacksInfo for HandleSubCommand.
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		SubCommand:       "clean",
		ComponentType:    "terraform",
		DryRun:           opts.DryRun,
	}

	// Build AdditionalArgsAndFlags for backward compatibility with HandleSubCommand.
	if opts.Force {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, ForceFlag)
	}
	if opts.Everything {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, EverythingFlag)
	}
	if opts.SkipLockFile {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, SkipTerraformLockFileFlag)
	}

	// Resolve component name via ProcessStacks when a component is provided.
	componentPath := atmosConfig.TerraformDirAbsolutePath
	if opts.Component != "" {
		resolvedInfo, err := s.processor.ProcessStacks(atmosConfig, info)
		if err != nil {
			return err
		}
		info = resolvedInfo

		// Use resolved terraform component path.
		terraformComponent := info.Context.BaseComponent
		if terraformComponent == "" {
			terraformComponent = opts.Component
		}
		componentPath = filepath.Join(atmosConfig.TerraformDirAbsolutePath, terraformComponent)
	}

	return s.HandleSubCommand(&info, componentPath, atmosConfig)
}

// cleanPluginCache cleans the Terraform plugin cache directory.
func (s *Service) cleanPluginCache(opts *Options, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "clean.cleanPluginCache")()

	// Get the plugin cache directory path.
	// First check if user has a custom plugin cache configured.
	pluginCacheDir := atmosConfig.Components.Terraform.PluginCacheDir

	// Use XDG cache directory if no custom path configured.
	if pluginCacheDir == "" {
		cacheDir, err := xdg.GetXDGCacheDir("terraform/plugins", xdg.DefaultCacheDirPerm)
		if err != nil {
			return fmt.Errorf("failed to get plugin cache directory: %w", err)
		}
		pluginCacheDir = cacheDir
	}

	// Check if the directory exists.
	if _, err := os.Stat(pluginCacheDir); os.IsNotExist(err) {
		_ = ui.Writeln("")
		_ = ui.Success("Plugin cache directory does not exist, nothing to clean")
		_ = ui.Writeln("")
		return nil
	}

	// Dry run mode.
	if opts.DryRun {
		_ = ui.Writeln("Dry run mode: the following directory would be deleted:")
		_ = ui.Writeln(fmt.Sprintf("  %s", pluginCacheDir))
		return nil
	}

	// Prompt for confirmation unless --force is set.
	if !opts.Force {
		message := fmt.Sprintf("This will delete the Terraform plugin cache directory: %s", pluginCacheDir)
		_ = ui.Writeln(message)
		_ = ui.Writeln("")
		confirmed, err := confirmDeletion()
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	// Delete the plugin cache directory.
	if err := os.RemoveAll(pluginCacheDir); err != nil {
		return fmt.Errorf("failed to delete plugin cache directory: %w", err)
	}

	_ = ui.Writeln("")
	_ = ui.Success(fmt.Sprintf("Deleted plugin cache directory: %s", pluginCacheDir))
	_ = ui.Writeln("")
	return nil
}

// HandleSubCommand handles the 'clean' subcommand logic.
func (s *Service) HandleSubCommand(info *schema.ConfigAndStacksInfo, componentPath string, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "clean.HandleSubCommand")()

	if info.SubCommand != "clean" {
		return nil
	}

	cleanPath, err := s.buildCleanPath(info, componentPath)
	if err != nil {
		return err
	}

	relativePath, err := buildRelativePath(atmosConfig.BasePath, componentPath, info.Context.BaseComponent)
	if err != nil {
		return err
	}

	force := slices.Contains(info.AdditionalArgsAndFlags, ForceFlag)
	filesToClear := s.initializeFilesToClear(*info, atmosConfig)

	folders, err := s.collectAllFolders(info, atmosConfig, cleanPath, filesToClear)
	if err != nil {
		return err
	}

	tfDataDirFolders, tfDataDir := collectTFDataDirFolders(cleanPath)
	total := countFilesToDelete(folders, tfDataDirFolders)

	if total == 0 {
		_ = ui.Writeln("")
		_ = ui.Success("Nothing to delete")
		_ = ui.Writeln("")
		return nil
	}

	if info.DryRun {
		printDryRunOutput(folders, tfDataDirFolders, atmosConfig.BasePath, total)
		return nil
	}

	if !force {
		message := buildConfirmationMessage(info, total)
		if confirmed, err := promptForConfirmation(tfDataDirFolders, tfDataDir, message); err != nil || !confirmed {
			return err
		}
	}

	executeCleanDeletion(folders, tfDataDirFolders, relativePath, atmosConfig)
	return nil
}

// buildCleanPath determines the path to clean based on component info.
func (s *Service) buildCleanPath(info *schema.ConfigAndStacksInfo, componentPath string) (string, error) {
	if info.ComponentFromArg != "" && info.StackFromArg == "" {
		if info.Context.BaseComponent == "" {
			return "", fmt.Errorf("%w: %s", ErrComponentNotFound, info.ComponentFromArg)
		}
		return filepath.Join(componentPath, info.Context.BaseComponent), nil
	}
	return componentPath, nil
}

// buildRelativePath creates the relative path for display purposes.
func buildRelativePath(basePath, componentPath string, baseComponent string) (string, error) {
	relativePath, err := getRelativePath(basePath, componentPath)
	if err != nil {
		return "", err
	}
	if baseComponent != "" {
		relativePath = strings.Replace(relativePath, baseComponent, "", 1)
		relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))
	}
	return relativePath, nil
}

// initializeFilesToClear builds the list of file patterns to delete.
//
//nolint:gocritic // hugeParam: value type is required as info is modified within the function
func (s *Service) initializeFilesToClear(info schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) []string {
	if info.ComponentFromArg == "" {
		return []string{TerraformDir, TerraformLockFile, TerraformVarFileGlob, TerraformStateDir}
	}
	varFile := s.processor.ConstructTerraformComponentVarfileName(&info)
	planFile := s.processor.ConstructTerraformComponentPlanfileName(&info)
	files := []string{TerraformDir, varFile, planFile}

	if !slices.Contains(info.AdditionalArgsAndFlags, SkipTerraformLockFileFlag) {
		files = append(files, TerraformLockFile)
	}

	if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
		files = append(files, BackendConfigFile)
	}

	// Include auto-generated files from the generate section.
	if atmosConfig.Components.Terraform.AutoGenerateFiles {
		generateFiles := s.processor.GetGenerateFilenamesForComponent(info.ComponentSection)
		files = append(files, generateFiles...)
	}

	return files
}

// collectAllFolders collects all folders to clean including stack-specific folders.
func (s *Service) collectAllFolders(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration, cleanPath string, filesToClear []string) ([]Directory, error) {
	allComponentsRelativePaths, err := s.getComponentsToClean(info, atmosConfig)
	if err != nil {
		return nil, err
	}

	folders, err := s.processor.CollectComponentsDirectoryObjects(atmosConfig.TerraformDirAbsolutePath, allComponentsRelativePaths, filesToClear)
	if err != nil {
		log.Debug("error collecting folders and files", "error", err)
		return nil, err
	}

	if info.Component != "" && info.Stack != "" {
		if stackFolders, err := GetStackTerraformStateFolder(cleanPath, info.Stack); err != nil {
			log.Debug("error getting stack terraform state folders", "error", err)
		} else if stackFolders != nil {
			folders = append(folders, stackFolders...)
		}
	}
	return folders, nil
}

// getComponentsToClean determines which components should be cleaned.
func (s *Service) getComponentsToClean(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) ([]string, error) {
	if info.ComponentFromArg != "" {
		componentToClean := info.FinalComponent
		if componentToClean == "" && info.Context.BaseComponent != "" {
			componentToClean = info.Context.BaseComponent
		}
		if componentToClean == "" {
			componentToClean = info.ComponentFromArg
		}
		log.Debug("Clean: Using component from arg", "ComponentFromArg", info.ComponentFromArg, "FinalComponent", info.FinalComponent, "BaseComponent", info.Context.BaseComponent, "componentToClean", componentToClean)
		return []string{componentToClean}, nil
	}

	log.Debug("Clean: No component from arg, calling ExecuteDescribeStacks", "StackFromArg", info.StackFromArg)
	stacksMap, err := s.processor.ExecuteDescribeStacks(atmosConfig, info.StackFromArg, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDescribeStack, err)
	}
	return s.processor.GetAllStacksComponentsPaths(stacksMap), nil
}

// countFilesToDelete counts the total number of files to delete.
func countFilesToDelete(folders []Directory, tfDataDirFolders []Directory) int {
	count := 0
	for _, folder := range folders {
		count += len(folder.Files)
	}
	for _, folder := range tfDataDirFolders {
		count += len(folder.Files)
	}
	return count
}

// printDryRunOutput prints what would be deleted in dry-run mode.
func printDryRunOutput(folders []Directory, tfDataDirFolders []Directory, basePath string, total int) {
	_ = ui.Writeln("Dry run mode: the following files would be deleted:")
	printFolderFiles(folders, basePath)
	printFolderFiles(tfDataDirFolders, basePath)
	_ = ui.Writeln(fmt.Sprintf("\nTotal: %d files would be deleted", total))
}

// printFolderFiles prints files from a list of folders.
func printFolderFiles(folders []Directory, basePath string) {
	for _, folder := range folders {
		for _, file := range folder.Files {
			fileRel, err := getRelativePath(basePath, file.FullPath)
			if err != nil {
				fileRel = file.Name
			}
			_ = ui.Writeln(fmt.Sprintf("  %s", fileRel))
		}
	}
}

// buildConfirmationMessage builds the confirmation message for deletion.
func buildConfirmationMessage(info *schema.ConfigAndStacksInfo, total int) string {
	if info.ComponentFromArg == "" {
		return fmt.Sprintf("This will delete %v local terraform state files affecting all components", total)
	}
	if info.Component != "" && info.Stack != "" {
		return fmt.Sprintf("This will delete %v local terraform state files for component '%s' in stack '%s'", total, info.Component, info.Stack)
	}
	if info.ComponentFromArg != "" {
		return fmt.Sprintf("This will delete %v local terraform state files for component '%s'", total, info.ComponentFromArg)
	}
	return "This will delete selected terraform state files"
}

// promptForConfirmation prompts user for confirmation and returns true if confirmed.
func promptForConfirmation(tfDataDirFolders []Directory, tfDataDir string, message string) (bool, error) {
	if len(tfDataDirFolders) > 0 {
		_ = ui.Writeln(fmt.Sprintf("Found ENV var %s=%s", EnvTFDataDir, tfDataDir))
		_ = ui.Writeln(fmt.Sprintf("Do you want to delete the folder '%s'? ", tfDataDir))
	}
	_ = ui.Writeln(message)
	_ = ui.Writeln("")
	return confirmDeletion()
}

// confirmDeletion prompts the user for confirmation before deletion.
// If not in a TTY (e.g., CI/CD, tests), returns false to prevent deletion without explicit --force flag.
func confirmDeletion() (bool, error) {
	// Check if stdin is a TTY.
	// In non-interactive environments (tests, CI/CD), we should require --force flag.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		log.Debug("Not a TTY, skipping interactive confirmation (use --force to bypass)")
		return false, errUtils.ErrInteractiveNotAvailable
	}

	message := "Are you sure?"
	confirm, err := confirmDeleteTerraformLocal(message)
	if err != nil {
		return false, err
	}
	if !confirm {
		log.Warn("Mission aborted.")
		return false, nil
	}
	return true, nil
}

// confirmDeleteTerraformLocal shows a confirmation dialog.
func confirmDeleteTerraformLocal(message string) (confirm bool, err error) {
	confirm = false
	t := utils.NewAtmosHuhTheme()
	confirmPrompt := huh.NewConfirm().
		Title(message).
		Affirmative("Yes!").
		Negative("No.").
		Value(&confirm).WithTheme(t)
	if err := confirmPrompt.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return confirm, fmt.Errorf("%w", errUtils.ErrUserAborted)
		}
		return confirm, err
	}

	return confirm, nil
}
