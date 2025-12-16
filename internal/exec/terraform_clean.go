package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	tuiTerm "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/internal/tui/utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// EnvTFDataDir is the environment variable name for TF_DATA_DIR.
const EnvTFDataDir = "TF_DATA_DIR"

var (
	ErrParseTerraformComponents  = errors.New("could not parse Terraform components")
	ErrParseComponentsAttributes = errors.New("could not parse component attributes")
	ErrDescribeStack             = errors.New("error describe stacks")
	ErrEmptyPath                 = errors.New("path cannot be empty")
	ErrPathNotExist              = errors.New("path not exist")
	ErrFileStat                  = errors.New("error get file stat")
	ErrMatchPattern              = errors.New("error matching pattern")
	ErrReadDir                   = errors.New("error reading directory")
	ErrFailedFoundStack          = errors.New("failed to find stack folders")
	ErrCollectFiles              = errors.New("failed to collect files")
	ErrEmptyEnvDir               = errors.New("ENV TF_DATA_DIR is empty")
	ErrResolveEnvDir             = errors.New("error resolving TF_DATA_DIR path")
	ErrRefusingToDeleteDir       = errors.New("refusing to delete root directory")
	ErrRefusingToDelete          = errors.New("refusing to delete directory containing")
	ErrRootPath                  = errors.New("root path cannot be empty")
	ErrUserAborted               = errors.New("mission aborted")
	ErrComponentNotFound         = errors.New("could not find component")
)

type ObjectInfo struct {
	FullPath     string
	RelativePath string
	Name         string
	IsDir        bool
}

type Directory struct {
	Name         string
	FullPath     string
	RelativePath string
	Files        []ObjectInfo
}

// findFoldersNamesWithPrefix finds the names of folders that match the given prefix under the specified root path.
// The search is performed at the root level (level 1) and one level deeper (level 2).
func findFoldersNamesWithPrefix(root, prefix string) ([]string, error) {
	var folderNames []string
	if root == "" {
		return nil, ErrRootPath
	}
	// First, read the directories at the root level (level 1)
	level1Dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("%w path %s: error %v", ErrReadDir, root, err)
	}

	for _, dir := range level1Dirs {
		if dir.IsDir() {
			// If the directory at level 1 matches the prefix, add it
			if prefix == "" || strings.HasPrefix(dir.Name(), prefix) {
				folderNames = append(folderNames, dir.Name())
			}

			// Now, explore one level deeper (level 2)
			level2Path := filepath.Join(root, dir.Name())
			level2Dirs, err := os.ReadDir(level2Path)
			if err != nil {
				log.Debug("Error reading subdirectory", "directory", level2Path, "error", err)
				continue
			}

			for _, subDir := range level2Dirs {
				if subDir.IsDir() && (prefix == "" || strings.HasPrefix(subDir.Name(), prefix)) {
					folderNames = append(folderNames, filepath.Join(dir.Name(), subDir.Name()))
				}
			}
		}
	}

	return folderNames, nil
}

func CollectDirectoryObjects(basePath string, patterns []string) ([]Directory, error) {
	defer perf.Track(nil, "exec.CollectDirectoryObjects")()

	if basePath == "" {
		return nil, ErrEmptyPath
	}
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w %s", ErrPathNotExist, basePath)
	}
	var folders []Directory

	// Helper function to add file information if it exists
	addFileInfo := func(filePath string) (*ObjectInfo, error) {
		relativePath, err := filepath.Rel(basePath, filePath)
		if err != nil {
			return nil, fmt.Errorf("%w  %s: %v", ErrRelPath, filePath, err)
		}
		info, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			return nil, nil // Skip if the file doesn't exist
		} else if err != nil {
			return nil, fmt.Errorf("%w,path %s error %v", ErrFileStat, filePath, err)
		}

		return &ObjectInfo{
			FullPath:     filePath,
			RelativePath: relativePath,
			Name:         filepath.Base(filePath),
			IsDir:        info.IsDir(),
		}, nil
	}

	// Function to create a folder entry with its files
	createFolder := func(folderPath string, folderName string) (*Directory, error) {
		relativePath, err := filepath.Rel(basePath, folderPath)
		if err != nil {
			return nil, fmt.Errorf("%w %s: %v", ErrRelPath, folderPath, err)
		}

		return &Directory{
			Name:         folderName,
			FullPath:     folderPath,
			RelativePath: relativePath,
			Files:        []ObjectInfo{},
		}, nil
	}

	// Function to collect files for a given folder path
	collectFilesInFolder := func(folder *Directory, folderPath string) error {
		for _, pat := range patterns {
			matchedFiles, err := filepath.Glob(filepath.Join(folderPath, pat))
			if err != nil {
				return fmt.Errorf("%w %s in folder %s: %v", ErrMatchPattern, pat, folderPath, err)
			}

			// Add matched files to folder
			for _, matchedFile := range matchedFiles {
				fileInfo, err := addFileInfo(matchedFile)
				if err != nil {
					return err
				}
				if fileInfo != nil {
					folder.Files = append(folder.Files, *fileInfo)
				}
			}
		}
		return nil
	}

	// Collect files for the base path itself
	baseFolder, err := createFolder(basePath, filepath.Base(basePath))
	if err != nil {
		return nil, err
	}
	err = collectFilesInFolder(baseFolder, basePath)
	if err != nil {
		return nil, err
	}
	if len(baseFolder.Files) > 0 {
		folders = append(folders, *baseFolder)
	}

	// Now, search for folders and their files from immediate subdirectories
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("error reading the base path %s: %v", basePath, err)
	}

	for _, entry := range entries {
		// Only proceed if the entry is a directory
		if entry.IsDir() {
			subDirPath := filepath.Join(basePath, entry.Name())

			// Create the folder entry
			folder, err := createFolder(subDirPath, entry.Name())
			if err != nil {
				return nil, err
			}

			// Collect files in the subdirectory
			err = collectFilesInFolder(folder, subDirPath)
			if err != nil {
				return nil, err
			}

			// Add folder to the list only if it contains files
			if len(folder.Files) > 0 {
				folders = append(folders, *folder)
			}
		}
	}

	return folders, nil
}

// get stack terraform state files
func getStackTerraformStateFolder(componentPath string, stack string) ([]Directory, error) {
	tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d")
	tfStateFolderNames, err := findFoldersNamesWithPrefix(tfStateFolderPath, stack)
	if err != nil {
		return nil, fmt.Errorf("%w : %v", ErrFailedFoundStack, err)
	}
	var stackTfStateFolders []Directory
	for _, folderName := range tfStateFolderNames {
		tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d", folderName)
		// Check if exists
		if _, err := os.Stat(tfStateFolderPath); os.IsNotExist(err) {
			continue
		}
		directories, err := CollectDirectoryObjects(tfStateFolderPath, []string{"*.tfstate", "*.tfstate.backup"})
		if err != nil {
			return nil, fmt.Errorf("%w in %s: %v", ErrCollectFiles, tfStateFolderPath, err)
		}
		for i := range directories {
			if directories[i].Files != nil {
				for j := range directories[i].Files {
					directories[i].Files[j].Name = folderName + "/" + directories[i].Files[j].Name
				}
			}
		}
		stackTfStateFolders = append(stackTfStateFolders, directories...)
	}

	return stackTfStateFolders, nil
}

// getRelativePath computes the relative path from basePath to componentPath.
func getRelativePath(basePath, componentPath string) (string, error) {
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	absComponentPath, err := filepath.Abs(componentPath)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(absBasePath, absComponentPath)
	if err != nil {
		return "", err
	}

	return filepath.Base(absBasePath) + "/" + relPath, nil
}

func confirmDeleteTerraformLocal(message string) (confirm bool, err error) {
	confirm = false
	t := utils.NewAtmosHuhTheme()
	confirmPrompt := huh.NewConfirm().
		Title(message).
		Affirmative("Yes!").
		Negative("No.").
		Value(&confirm).WithTheme(t)
	if err := confirmPrompt.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return confirm, fmt.Errorf("%w", ErrUserAborted)
		}
		return confirm, err
	}

	return confirm, nil
}

// DeletePathTerraform deletes the specified file or folder with a checkmark or xmark.
func DeletePathTerraform(fullPath string, objectName string) error {
	defer perf.Track(nil, "exec.DeletePathTerraform")()

	// Normalize path separators to forward slashes for consistent output across platforms
	normalizedObjectName := filepath.ToSlash(objectName)

	fileInfo, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		_ = ui.Errorf("Cannot delete %s: path does not exist", normalizedObjectName)
		return err
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrRefuseDeleteSymbolicLink, normalizedObjectName)
	}
	// Proceed with deletion
	err = os.RemoveAll(fullPath)
	if err != nil {
		_ = ui.Errorf("Error deleting %s", normalizedObjectName)
		return err
	}
	_ = ui.Successf("Deleted %s", normalizedObjectName)
	return nil
}

// confirmDeletion prompts the user for confirmation before deletion.
// If not in a TTY (e.g., CI/CD, tests), returns false to prevent deletion without explicit --force flag.
func confirmDeletion() (bool, error) {
	// Check if stdin is a TTY
	// In non-interactive environments (tests, CI/CD), we should require --force flag
	if !tuiTerm.IsTTYSupportForStdin() {
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

// deleteFolders handles the deletion of the specified folders and files.
func deleteFolders(folders []Directory, relativePath string, atmosConfig *schema.AtmosConfiguration) {
	var errors []error
	for _, folder := range folders {
		for _, file := range folder.Files {
			fileRel, err := getRelativePath(atmosConfig.BasePath, file.FullPath)
			if err != nil {
				log.Debug("Failed to get relative path", "path", file.FullPath, "error", err)
				fileRel = filepath.Join(relativePath, file.Name)
			}
			if file.IsDir {
				if err := DeletePathTerraform(file.FullPath, fileRel+"/"); err != nil {
					errors = append(errors, fmt.Errorf("failed to delete %s: %w", fileRel, err))
				}
			} else {
				if err := DeletePathTerraform(file.FullPath, fileRel); err != nil {
					errors = append(errors, fmt.Errorf("failed to delete %s: %w", fileRel, err))
				}
			}
		}
	}
	if len(errors) > 0 {
		for _, err := range errors {
			log.Debug(err)
		}
	}
	// check if the folder is empty by using the os.ReadDir function
	for _, folder := range folders {
		entries, err := os.ReadDir(folder.FullPath)
		if err == nil && len(entries) == 0 {
			if err := os.Remove(folder.FullPath); err != nil {
				log.Debug("Error removing directory", "path", folder.FullPath, "error", err)
			}
		}
	}
}

// handleTFDataDir handles the deletion of the TF_DATA_DIR if specified.
func handleTFDataDir(componentPath string, relativePath string) {
	tfDataDir := os.Getenv(EnvTFDataDir) //nolint:forbidigo // TF_DATA_DIR is a Terraform runtime env var, not an Atmos config option.
	if tfDataDir == "" {
		return
	}
	if err := IsValidDataDir(tfDataDir); err != nil {
		log.Debug("Error validating TF_DATA_DIR", "error", err)
		return
	}
	if _, err := os.Stat(filepath.Join(componentPath, tfDataDir)); os.IsNotExist(err) {
		log.Debug("TF_DATA_DIR does not exist", EnvTFDataDir, tfDataDir, "error", err)
		return
	}
	if err := DeletePathTerraform(filepath.Join(componentPath, tfDataDir), filepath.Join(relativePath, tfDataDir)); err != nil {
		log.Debug("Error deleting TF_DATA_DIR", EnvTFDataDir, tfDataDir, "error", err)
	}
}

func initializeFilesToClear(info schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) []string {
	if info.ComponentFromArg == "" {
		return []string{".terraform", ".terraform.lock.hcl", "*.tfvar.json", "terraform.tfstate.d"}
	}
	varFile := constructTerraformComponentVarfileName(&info)
	planFile := constructTerraformComponentPlanfileName(&info)
	files := []string{".terraform", varFile, planFile}

	if !slices.Contains(info.AdditionalArgsAndFlags, skipTerraformLockFileFlag) {
		files = append(files, ".terraform.lock.hcl")
	}

	if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
		files = append(files, "backend.tf.json")
	}

	// Include auto-generated files from the generate section.
	if atmosConfig.Components.Terraform.AutoGenerateFiles {
		generateFiles := GetGenerateFilenamesForComponent(info.ComponentSection)
		files = append(files, generateFiles...)
	}

	return files
}

func IsValidDataDir(tfDataDir string) error {
	defer perf.Track(nil, "exec.IsValidDataDir")()

	if tfDataDir == "" {
		return ErrEmptyEnvDir
	}
	absTFDataDir, err := filepath.Abs(tfDataDir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrResolveEnvDir, err)
	}

	// Check for root path on both Unix and Windows systems
	if absTFDataDir == "/" || absTFDataDir == filepath.Clean("/") {
		return fmt.Errorf("%w: %s", ErrRefusingToDeleteDir, absTFDataDir)
	}

	// Windows-specific root path check (like C:\ or D:\)
	if len(absTFDataDir) == 3 && absTFDataDir[1:] == ":\\" {
		return fmt.Errorf("%w: %s", ErrRefusingToDeleteDir, absTFDataDir)
	}

	if strings.Contains(absTFDataDir, "..") {
		return fmt.Errorf("%w: %s", ErrRefusingToDelete, "..")
	}
	return nil
}

// ExecuteClean cleans up Terraform state and artifacts for a component.
func ExecuteClean(opts *CleanOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteClean")()

	if opts == nil {
		return errUtils.ErrNilParam
	}

	log.Debug("ExecuteClean called",
		"component", opts.Component,
		"stack", opts.Stack,
		"force", opts.Force,
		"everything", opts.Everything,
		"skipLockFile", opts.SkipLockFile,
		"dryRun", opts.DryRun,
		"cache", opts.Cache,
	)

	// Handle plugin cache cleanup if --cache flag is set.
	if opts.Cache {
		if err := cleanPluginCache(opts.Force, opts.DryRun); err != nil {
			return err
		}
	}

	// Build ConfigAndStacksInfo for HandleCleanSubCommand.
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		SubCommand:       "clean",
		ComponentType:    "terraform",
		DryRun:           opts.DryRun,
	}

	// Build AdditionalArgsAndFlags for backward compatibility with HandleCleanSubCommand.
	if opts.Force {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, forceFlag)
	}
	if opts.Everything {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, everythingFlag)
	}
	if opts.SkipLockFile {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, skipTerraformLockFileFlag)
	}

	// Resolve component name via ProcessStacks when a component is provided.
	// This matches the behavior from main where clean went through ExecuteTerraform() -> ProcessStacks().
	// ProcessStacks resolves Atmos component names (e.g., "mycomponent") to actual Terraform
	// component directories (e.g., "mock") via the metadata.component field in stack config.
	componentPath := atmosConfig.TerraformDirAbsolutePath
	if opts.Component != "" {
		// shouldCheckStack = only require stack if explicitly provided (matching main's behavior).
		shouldCheckStack := opts.Stack != ""
		resolvedInfo, err := ProcessStacks(atmosConfig, info, shouldCheckStack, false, false, nil, nil)
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

	return HandleCleanSubCommand(&info, componentPath, atmosConfig)
}

// buildCleanPath determines the path to clean based on component info.
func buildCleanPath(info *schema.ConfigAndStacksInfo, componentPath string) (string, error) {
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
		relativePath = strings.TrimPrefix(relativePath, "/")
	}
	return relativePath, nil
}

// getComponentsToClean determines which components should be cleaned.
func getComponentsToClean(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) ([]string, error) {
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
	var filterComponents []string
	if info.ComponentFromArg != "" {
		filterComponents = append(filterComponents, info.ComponentFromArg)
	}
	stacksMap, err := ExecuteDescribeStacks(
		atmosConfig,
		info.StackFromArg,
		filterComponents,
		nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDescribeStack, err)
	}
	return getAllStacksComponentsPaths(stacksMap), nil
}

// collectTFDataDirFolders collects folders from TF_DATA_DIR environment variable.
func collectTFDataDirFolders(cleanPath string) ([]Directory, string) {
	tfDataDir := os.Getenv(EnvTFDataDir) //nolint:forbidigo // TF_DATA_DIR is a Terraform runtime env var, not an Atmos config option.
	if tfDataDir == "" {
		return nil, ""
	}

	if err := IsValidDataDir(tfDataDir); err != nil {
		log.Debug("error validating TF_DATA_DIR", "error", err)
		return nil, tfDataDir
	}

	folders, err := CollectDirectoryObjects(cleanPath, []string{tfDataDir})
	if err != nil {
		log.Debug("error collecting folder of ENV TF_DATA_DIR", "error", err)
		return nil, tfDataDir
	}
	return folders, tfDataDir
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

// collectAllFolders collects all folders to clean including stack-specific folders.
func collectAllFolders(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration, cleanPath string, filesToClear []string) ([]Directory, error) {
	allComponentsRelativePaths, err := getComponentsToClean(info, atmosConfig)
	if err != nil {
		return nil, err
	}

	folders, err := CollectComponentsDirectoryObjects(atmosConfig.TerraformDirAbsolutePath, allComponentsRelativePaths, filesToClear)
	if err != nil {
		log.Debug("error collecting folders and files", "error", err)
		return nil, err
	}

	if info.Component != "" && info.Stack != "" {
		if stackFolders, err := getStackTerraformStateFolder(cleanPath, info.Stack); err != nil {
			log.Debug("error getting stack terraform state folders", "error", err)
		} else if stackFolders != nil {
			folders = append(folders, stackFolders...)
		}
	}
	return folders, nil
}

// executeCleanDeletion performs the actual deletion of folders.
func executeCleanDeletion(folders []Directory, tfDataDirFolders []Directory, relativePath string, atmosConfig *schema.AtmosConfiguration) {
	deleteFolders(folders, relativePath, atmosConfig)
	if len(tfDataDirFolders) > 0 {
		handleTFDataDir(tfDataDirFolders[0].FullPath, relativePath)
	}
}

// HandleCleanSubCommand handles the 'clean' subcommand logic.
func HandleCleanSubCommand(info *schema.ConfigAndStacksInfo, componentPath string, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.HandleCleanSubCommand")()

	if info.SubCommand != "clean" {
		return nil
	}

	cleanPath, err := buildCleanPath(info, componentPath)
	if err != nil {
		return err
	}

	relativePath, err := buildRelativePath(atmosConfig.BasePath, componentPath, info.Context.BaseComponent)
	if err != nil {
		return err
	}

	force := slices.Contains(info.AdditionalArgsAndFlags, forceFlag)
	filesToClear := initializeFilesToClear(*info, atmosConfig)

	folders, err := collectAllFolders(info, atmosConfig, cleanPath, filesToClear)
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

// cleanPluginCache cleans the Terraform plugin cache directory.
func cleanPluginCache(force, dryRun bool) error {
	defer perf.Track(nil, "exec.cleanPluginCache")()

	// Get XDG cache directory for terraform plugins.
	cacheDir, err := xdg.GetXDGCacheDir("terraform/plugins", xdg.DefaultCacheDirPerm)
	if err != nil {
		log.Warn("Failed to determine plugin cache directory", "error", err)
		return nil
	}

	// Check if cache directory exists.
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		_ = ui.Success("Plugin cache directory does not exist, nothing to clean")
		return nil
	}

	if dryRun {
		_ = ui.Writef("Dry run mode: would delete plugin cache directory: %s\n", cacheDir)
		return nil
	}

	// Prompt for confirmation unless --force is set.
	if !force {
		_ = ui.Writef("This will delete the plugin cache directory: %s\n", cacheDir)
		confirmed, err := confirmDeletion()
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	// Remove the cache directory.
	if err := os.RemoveAll(cacheDir); err != nil {
		log.Warn("Failed to clean plugin cache", "path", cacheDir, "error", err)
		return err
	}

	_ = ui.Successf("Cleaned plugin cache: %s", cacheDir)
	return nil
}
