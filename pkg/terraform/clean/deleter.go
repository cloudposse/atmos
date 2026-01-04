package clean

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// DeletePath deletes the specified file or folder with a checkmark or xmark.
func DeletePath(fullPath string, objectName string) error {
	defer perf.Track(nil, "clean.DeletePath")()

	// Normalize path separators to forward slashes for consistent output across platforms.
	normalizedObjectName := filepath.ToSlash(objectName)

	fileInfo, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		_ = ui.Errorf("Cannot delete %s: path does not exist", normalizedObjectName)
		return err
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrRefuseDeleteSymbolicLink, normalizedObjectName)
	}
	// Proceed with deletion.
	err = os.RemoveAll(fullPath)
	if err != nil {
		_ = ui.Errorf("Error deleting %s", normalizedObjectName)
		return err
	}
	_ = ui.Successf("Deleted %s", normalizedObjectName)
	return nil
}

// deleteFolders handles the deletion of the specified folders and files.
//
//nolint:gocognit,revive // complexity is inherent in iterating directories/files with error handling and reporting
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
				if err := DeletePath(file.FullPath, fileRel+"/"); err != nil {
					errors = append(errors, fmt.Errorf("failed to delete %s: %w", fileRel, err))
				}
			} else {
				if err := DeletePath(file.FullPath, fileRel); err != nil {
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
	// Check if the folder is empty by using the os.ReadDir function.
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
	if err := DeletePath(filepath.Join(componentPath, tfDataDir), filepath.Join(relativePath, tfDataDir)); err != nil {
		log.Debug("Error deleting TF_DATA_DIR", EnvTFDataDir, tfDataDir, "error", err)
	}
}

// executeCleanDeletion performs the actual deletion of folders.
func executeCleanDeletion(folders []Directory, tfDataDirFolders []Directory, relativePath string, atmosConfig *schema.AtmosConfiguration) {
	deleteFolders(folders, relativePath, atmosConfig)
	if len(tfDataDirFolders) > 0 {
		handleTFDataDir(tfDataDirFolders[0].FullPath, relativePath)
	}
}
