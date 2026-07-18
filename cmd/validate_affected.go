package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/validation"
)

func addAffectedValidationFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("affected", false, "Validate only files affected since the Git merge-base")
	cmd.Flags().String("base", "", "Git base ref or SHA to compare against for affected validation")
}

// validationAffectedFiles resolves changed files only when --affected is set.
func validationAffectedFiles(cmd *cobra.Command) ([]string, bool, error) {
	flag := cmd.Flags().Lookup("affected")
	if flag == nil || !flag.Changed {
		return nil, false, nil
	}
	affected, err := cmd.Flags().GetBool("affected")
	if err != nil || !affected {
		return nil, affected, err
	}
	base, err := cmd.Flags().GetString("base")
	if err != nil {
		return nil, false, err
	}
	paths, err := validation.AffectedFiles(base)
	if err != nil {
		return nil, false, err
	}
	return paths, true, nil
}

func affectedPathExists(path string) bool {
	_, err := os.Stat(filepath.FromSlash(path))
	return err == nil
}

func affectedPathsWithExistingFiles(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if affectedPathExists(path) {
			result = append(result, path)
		}
	}
	return result
}

func isAtmosConfigPath(path string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "atmos.yaml" || path == "atmos.yml" || path == ".atmos.yaml" || path == ".atmos.yml" {
		return true
	}
	for _, directory := range []string{"atmos.d/", ".atmos.d/", "profiles/", ".atmos/profiles/"} {
		if strings.HasPrefix(path, directory) {
			return true
		}
	}
	return false
}

func affectedPathsContainAtmosConfig(paths []string) bool {
	for _, path := range paths {
		if isAtmosConfigPath(path) {
			return true
		}
	}
	return false
}

func affectedPathIsWithin(path string, directory string) bool {
	if directory == "" {
		return false
	}
	absPath, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(directory, absPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func affectedStacksApplicable(paths []string) bool {
	if affectedPathsContainAtmosConfig(paths) {
		return true
	}
	for _, path := range paths {
		if affectedPathIsWithin(path, atmosConfig.StacksBaseAbsolutePath) {
			return true
		}
	}
	return false
}

func affectedWorkflowPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := filepath.ToSlash(filepath.Clean(path))
		if strings.HasPrefix(normalized, ".github/workflows/") && (strings.HasSuffix(normalized, ".yaml") || strings.HasSuffix(normalized, ".yml")) && affectedPathExists(normalized) {
			result = append(result, normalized)
		}
	}
	return result
}

func affectedWorkflowConfigChanged(paths []string) bool {
	for _, path := range paths {
		normalized := filepath.ToSlash(filepath.Clean(path))
		if normalized == ".github/actionlint.yaml" || normalized == ".github/actionlint.yml" {
			return true
		}
	}
	return false
}

func affectedEditorConfigFiles(paths []string) ([]string, bool) {
	for _, path := range paths {
		normalized := filepath.ToSlash(filepath.Clean(path))
		if filepath.Base(normalized) == ".editorconfig" || normalized == ".editorconfig-checker.json" || normalized == ".ecrc" {
			// EditorConfig rule changes can affect every tracked source file.
			return nil, true
		}
	}
	return affectedPathsWithExistingFiles(paths), false
}

func affectedSchemaFiles(paths []string, sourceKey string) ([]string, bool) {
	if sourceKey == validateConfigSchemaKey {
		files := make([]string, 0, len(paths))
		for _, path := range paths {
			if isAtmosConfigPath(path) && affectedPathExists(path) {
				files = append(files, path)
			}
		}
		return files, false
	}

	for _, path := range paths {
		if isAtmosConfigPath(path) || strings.EqualFold(filepath.Ext(path), ".json") {
			// A changed configuration can alter the schema registry, and a changed
			// schema can change the validity of every matched YAML file.
			return nil, true
		}
	}

	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if affectedPathExists(path) && (strings.EqualFold(filepath.Ext(path), ".yaml") || strings.EqualFold(filepath.Ext(path), ".yml")) {
			files = append(files, path)
		}
	}
	return files, false
}

func validationNoAffectedFiles(cmd *cobra.Command, validator string) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "No affected %s files to validate.\n", validator)
	return err
}
