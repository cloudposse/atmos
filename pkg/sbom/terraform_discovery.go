package sbom

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// findTerraformConfigurations includes initialized module-only roots, which
// Terraform does not give a provider lock file. Never descend into downloaded
// modules: their dependency evidence belongs to the owning root's graph.
func findTerraformConfigurations(base string, locks []string) ([]string, error) {
	directories := make(map[string]bool)
	for _, lock := range locks {
		directories[filepath.Dir(lock)] = true
	}
	err := filepath.WalkDir(base, terraformConfigurationVisitor(directories))
	result := make([]string, 0, len(directories))
	for directory := range directories {
		result = append(result, directory)
	}
	sort.Strings(result)
	return result, err
}

func hasModuleManifest(directory string) (bool, error) {
	info, err := os.Stat(filepath.Join(directory, "modules", "modules.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}

func terraformConfigurationVisitor(directories map[string]bool) fs.WalkDirFunc {
	return func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		switch entry.Name() {
		case ".git":
			return filepath.SkipDir
		case ".terraform":
			initialized, err := hasModuleManifest(path)
			if err != nil {
				return err
			}
			if initialized {
				directories[filepath.Dir(path)] = true
			}
			return filepath.SkipDir
		}
		return nil
	}
}
