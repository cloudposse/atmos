package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	ciplanfile "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
)

// configurationWorkdirDigest binds retained module/configuration bytes, excluding mutable execution data.
func configurationWorkdirDigest(root, planPath string) (string, error) {
	return workdirDigest(root, planPath, nil)
}

// workdirDigest hashes the retained files below root, additionally skipping the top-level files
// that ignore selects by their slash-separated path relative to root.
func workdirDigest(root, planPath string, ignore func(relative string) bool) (string, error) {
	// An explicit plan path may be absolute while the directory is relative, or the reverse.
	// Both must compare equal, or the plan file itself would be hashed into the directory digest.
	root, planPath, err := absolutePaths(root, planPath)
	if err != nil {
		return "", err
	}
	files := make(map[string]string)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipWorkdirDirectory(root, path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipWorkdirFile(root, path, planPath, entry.Name(), ignore) {
			return nil
		}
		return recordWorkdirFile(root, path, entry, files)
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func skipWorkdirFile(root, path, planPath, name string, ignore func(string) bool) bool {
	if path == planPath || path == planPath+configurationPlanSuffix || insideTerraformCacheOutsideModules(root, path) {
		return true
	}
	return (!insideTerraformModules(root, path) && ignoredWorkdirFile(path, planPath, name)) || ignoredRelative(root, path, ignore)
}

// ignoredRelative applies a caller's filter to the slash-separated path relative to root.
func ignoredRelative(root, path string, ignore func(relative string) bool) bool {
	if ignore == nil {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && ignore(filepath.ToSlash(relative))
}

// absolutePaths returns both paths absolute and with symlinks resolved, so one file reached through
// two spellings (for example /var and /private/var on macOS) compares equal.
func absolutePaths(root, planPath string) (string, string, error) {
	absoluteRoot, err := resolvedPath(root)
	if err != nil {
		return "", "", err
	}
	absolutePlan, err := resolvedPath(planPath)
	if err != nil {
		return "", "", err
	}
	return absoluteRoot, absolutePlan, nil
}

func resolvedPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	return absolute, nil
}

// skipWorkdirDirectory reports whether a nested directory holds only mutable or cached data.
// The .atmos directory is provisioner bookkeeping (access timestamps and metadata lock files
// that even a read-only lookup of the retained directory creates), never executed content.
// Name-based exclusions apply only outside downloaded modules: a module's own subdirectories
// are content Terraform may execute during apply, whatever they are called.
func skipWorkdirDirectory(root, path, name string) bool {
	if path == root {
		return false
	}
	if insideTerraformCacheOutsideModules(root, path) {
		return true
	}
	return !insideTerraformModules(root, path) && (name == ".git" || name == ".terragrunt-cache" || name == ".atmos")
}

// Terraform's downloaded modules can contain scripts executed during apply. Other cache
// content, including providers and state, remains mutable and is excluded.
func insideTerraformCacheOutsideModules(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for i, part := range parts {
		if part == ".terraform" && i+1 < len(parts) && parts[i+1] != "modules" {
			return true
		}
	}
	return false
}

func insideTerraformModules(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for i, part := range parts {
		if part == ".terraform" && i+1 < len(parts) && parts[i+1] == "modules" {
			return true
		}
	}
	return false
}

// ignoredWorkdirFile reports whether a file is a plan, its binding, or mutable state. Plan files are
// reviewed artifacts, not configuration: besides the plan itself that includes an earlier plan
// still lying in the directory, the stored plan `deploy` downloads for comparison, and the fresh
// plan it generates.
func ignoredWorkdirFile(path, planPath, name string) bool {
	return path == planPath || strings.HasSuffix(name, planBindingSuffix) || strings.HasSuffix(name, ".planfile") ||
		name == ciplanfile.StoredPlanPrefix+ciplanfile.PlanFilename || name == freshPlanName ||
		strings.HasPrefix(name, ".atmos-plan-") || strings.HasPrefix(name, "terraform.tfstate")
}

// recordWorkdirFile digests one retained file by its slash-separated path relative to the root.
func recordWorkdirFile(root, path string, entry fs.DirEntry, files map[string]string) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: saved-plan workdir contains symlink", errUtils.ErrPluginUnsupportedExtension)
	}
	digest, err := fileDigest(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	files[filepath.ToSlash(relative)] = digest
	return nil
}
