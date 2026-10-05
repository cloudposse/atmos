package autoinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"github.com/cloudposse/atmos/pkg/terraform/clean"
	"github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

// moduleHCLPattern matches an HCL `module "..."` block header.
var moduleHCLPattern = regexp.MustCompile(`(?m)^\s*module\s+"`)

// providersMissing reports whether the lock file declares at least one provider dependency but
// neither the terraform-managed provider cache nor the legacy plugins directory has been
// populated under dataDir -- i.e. `terraform init` has never successfully installed providers for
// this data directory.
func providersMissing(dataDir, componentPath string) bool {
	providers, err := lockfile.ParseFile(filepath.Join(componentPath, lockfile.Name))
	if err != nil || len(providers) == 0 {
		return false
	}
	return !dirExists(filepath.Join(dataDir, "providers")) && !dirExists(filepath.Join(dataDir, "plugins"))
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
