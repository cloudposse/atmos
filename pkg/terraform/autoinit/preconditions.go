package autoinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/terraform/clean"
	"github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

const providerExecutablePerm = 0o111

// moduleHCLPattern matches an HCL `module "..."` block header.
var moduleHCLPattern = regexp.MustCompile(`(?m)^\s*module\s+"`)

// providersMissing checks the locked provider packages for the current platform. A directory
// alone is insufficient: an interrupted cleanup or an evicted shared cache can leave an empty
// package or a dangling symlink. Stat follows package/executable symlinks without hashing packages
// on every warm invocation; Terraform remains responsible for checksum verification.
func providersMissing(dataDir, componentPath string) bool {
	providers, err := lockfile.ParseFile(filepath.Join(componentPath, lockfile.Name))
	if err != nil || len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		if !providerInstalled(dataDir, provider) {
			return true
		}
	}
	return false
}

// providerInstalled accepts the current registry layout and the legacy platform plugin layout.
func providerInstalled(dataDir string, provider lockfile.Provider) bool {
	parts := strings.Split(provider.Source, "/")
	if len(parts) != 3 || provider.Version == "" {
		return false
	}
	prefix := "terraform-provider-" + parts[2] + "_v" + provider.Version
	dirs := providerPackageDirs(dataDir, provider)
	for i, dir := range dirs {
		if providerExecutablePresent(dir, prefix, i < 2) {
			return true
		}
	}
	// The engine may run under architecture emulation. A valid same-OS package must not
	// force perpetual init solely because Atmos itself has a different architecture. If the
	// engine cannot use that package, its diagnostic still triggers bounded runtime recovery.
	return alternativeProviderInstalled(dataDir, provider, prefix)
}

func alternativeProviderInstalled(dataDir string, provider lockfile.Provider, prefix string) bool {
	for _, dir := range alternateProviderPackageDirs(dataDir, provider) {
		if providerExecutablePresent(dir, prefix, true) {
			return true
		}
	}
	return false
}

func alternateProviderPackageDirs(dataDir string, provider lockfile.Provider) []string {
	native := providerPackageDirs(dataDir, provider)
	if len(native) == 0 {
		return nil
	}
	var dirs []string
	for _, path := range native[:2] {
		parent := filepath.Dir(path)
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Name() != filepath.Base(path) && strings.HasPrefix(entry.Name(), runtime.GOOS+"_") && !strings.HasSuffix(entry.Name(), ".lock") {
				dirs = append(dirs, filepath.Join(parent, entry.Name()))
			}
		}
	}
	return dirs
}

func providerPackageDirs(dataDir string, provider lockfile.Provider) []string {
	parts := strings.Split(provider.Source, "/")
	if len(parts) != 3 {
		return nil
	}
	for _, part := range append(parts, provider.Version) {
		if part == "" || part == "." || part == ".." || !filepath.IsLocal(part) || strings.ContainsAny(part, `/\\`) {
			return nil
		}
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	return []string{
		filepath.Join(dataDir, "providers", filepath.FromSlash(provider.Source), provider.Version, platform),
		filepath.Join(dataDir, "plugins", filepath.FromSlash(provider.Source), provider.Version, platform),
		filepath.Join(dataDir, "plugins", platform),
	}
}

func providerExecutablePresent(dir, prefix string, versionInPath bool) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !providerExecutableNameMatches(entry.Name(), prefix, versionInPath) {
			continue
		}
		info, statErr := os.Stat(filepath.Join(dir, entry.Name()))
		if statErr == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&providerExecutablePerm != 0) {
			return true
		}
	}
	return false
}

func providerExecutableNameMatches(name, prefix string, versionInPath bool) bool {
	base, _, _ := strings.Cut(prefix, "_v")
	if versionInPath && (name == base || name == base+".exe") {
		return true
	}
	suffix, matches := strings.CutPrefix(name, prefix)
	return matches && (suffix == "" || suffix == ".exe" || strings.HasPrefix(suffix, "_"))
}

// modulesMissing reports whether the component declares module blocks but dataDir has no modules
// manifest recorded from a prior init.
func modulesMissing(dataDir, componentPath string) bool {
	if fileExists(filepath.Join(dataDir, "modules", "modules.json")) {
		return false
	}
	return componentDeclaresModules(componentPath)
}

// componentDeclaresModules reports whether any root Terraform/OpenTofu configuration file in
// componentPath declares a module block, in either HCL or JSON syntax.
func componentDeclaresModules(componentPath string) bool {
	for _, pattern := range []string{"*.tf", "*.tofu"} {
		matches, _ := filepath.Glob(filepath.Join(componentPath, pattern))
		for _, f := range matches {
			content, err := os.ReadFile(f)
			if err == nil && moduleHCLPattern.Match(content) {
				return true
			}
		}
	}
	for _, pattern := range []string{"*.tf.json", "*.tofu.json"} {
		matches, _ := filepath.Glob(filepath.Join(componentPath, pattern))
		for _, f := range matches {
			if fileHasTopLevelModuleKey(f) {
				return true
			}
		}
	}
	return false
}

// fileHasTopLevelModuleKey reports whether the JSON document at path has a top-level "module"
// key.
func fileHasTopLevelModuleKey(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		return false
	}
	_, ok := doc["module"]
	return ok
}

// backendStateMissing reports whether the component configures a backend or cloud block but
// dataDir has no local state cache recorded from a prior successful init.
func backendStateMissing(dataDir, componentPath string) bool {
	if fileExists(filepath.Join(dataDir, "terraform.tfstate")) {
		return false
	}
	return componentDeclaresBackend(componentPath)
}

// componentDeclaresBackend reports whether componentPath has an auto-generated backend.tf.json,
// or any root Terraform/OpenTofu configuration file (.tf / .tf.json / .tofu / .tofu.json)
// declares a backend or cloud block. Discovery and JSON-vs-HCL detection reuse
// collectRootConfigFiles / isBackendConfigFile (fingerprint.go) so this agrees with what the
// fingerprint itself considers backend-relevant -- a narrower, HCL-only check here previously
// missed backend/cloud blocks declared in root *.tf.json / *.tofu.json files, which could make
// backendStateMissing wrongly report "no backend configured" (and Decide wrongly skip init) when
// no local terraform.tfstate exists yet.
func componentDeclaresBackend(componentPath string) bool {
	if fileExists(filepath.Join(componentPath, clean.BackendConfigFile)) {
		return true
	}
	files, err := collectRootConfigFiles(componentPath)
	if err != nil {
		return false
	}
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if isBackendConfigFile(filepath.Base(f), string(content)) {
			return true
		}
	}
	return false
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
