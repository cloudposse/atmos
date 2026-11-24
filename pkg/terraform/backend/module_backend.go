package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// backendConfigFileName holds the unit's backend settings for `init -backend-config` when the module
// declares its own backend block.
const backendConfigFileName = "atmos-backend.tfbackend"

// The terraformConfigSuffixes are the root module files Terraform reads; it ignores OpenTofu's `.tofu` files.
var terraformConfigSuffixes = []string{".tf", ".tf.json"}

// The tofuShadowedSuffixes map each OpenTofu-only suffix to the Terraform suffix it overrides: when both
// `name.tofu` and `name.tf` exist, OpenTofu reads only `name.tofu` (and likewise for `.tofu.json`).
var tofuShadowedSuffixes = map[string]string{".tofu": ".tf", ".tofu.json": ".tf.json"}

// moduleDeclaresBackend reports whether a root module file the selected engine reads, other than the host
// backend, declares a `backend` block inside a `terraform` block. A file that does not parse is skipped:
// Terraform reports it.
func moduleDeclaresBackend(directory string, openTofu bool) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	parser := hclparse.NewParser()
	for _, name := range engineConfigFiles(entries, openTofu) {
		if name != hostBackendName && fileDeclaresBackend(parser, filepath.Join(directory, name)) {
			return true
		}
	}
	return false
}

// engineConfigFiles returns the root module files the engine reads. Terraform reads `.tf` and `.tf.json`.
// OpenTofu also reads `.tofu` and `.tofu.json`, and skips a `.tf` or `.tf.json` file that has an OpenTofu
// counterpart with the same name.
func engineConfigFiles(entries []os.DirEntry, openTofu bool) []string {
	present := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			present[entry.Name()] = true
		}
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || ignoredConfigFile(name) {
			continue
		}
		if !openTofu {
			if configSuffix(name, terraformConfigSuffixes) != "" {
				files = append(files, name)
			}
			continue
		}
		if suffix := configSuffix(name, []string{".tofu.json", ".tofu"}); suffix != "" {
			files = append(files, name)
			continue
		}
		suffix := configSuffix(name, terraformConfigSuffixes)
		if suffix != "" && !shadowedByTofu(name, suffix, present) {
			files = append(files, name)
		}
	}
	return files
}

// ignoredConfigFile reports whether Terraform and OpenTofu skip a file when loading a module: hidden files,
// editor backups ending in `~`, and Emacs autosave files named `#...#`.
func ignoredConfigFile(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") ||
		(strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#"))
}

// configSuffix returns the first suffix of suffixes that name ends with, or "". The longer `.json` forms
// are listed first by callers that need to tell `.tofu.json` from `.tofu`.
func configSuffix(name string, suffixes []string) string {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return suffix
		}
	}
	return ""
}

// shadowedByTofu reports whether OpenTofu skips the Terraform file name because its OpenTofu counterpart exists.
func shadowedByTofu(name, suffix string, present map[string]bool) bool {
	base := strings.TrimSuffix(name, suffix)
	for tofuSuffix, shadowed := range tofuShadowedSuffixes {
		if shadowed == suffix && present[base+tofuSuffix] {
			return true
		}
	}
	return false
}

var (
	terraformBlockSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}}
	backendBlockSchema   = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "backend", LabelNames: []string{"type"}}},
	}
)

func fileDeclaresBackend(parser *hclparse.Parser, path string) bool {
	var file *hcl.File
	var diagnostics hcl.Diagnostics
	if strings.HasSuffix(path, ".json") {
		file, diagnostics = parser.ParseJSONFile(path)
	} else {
		file, diagnostics = parser.ParseHCLFile(path)
	}
	if diagnostics.HasErrors() || file == nil {
		return false
	}
	content, _, diagnostics := file.Body.PartialContent(terraformBlockSchema)
	if diagnostics.HasErrors() {
		return false
	}
	for _, block := range content.Blocks {
		nested, _, _ := block.Body.PartialContent(backendBlockSchema)
		if nested != nil && len(nested.Blocks) > 0 {
			return true
		}
	}
	return false
}

// passBackendToInit gives the unit's backend settings to `init` as a backend configuration file, as
// Terragrunt passes a `remote_state` without `generate` with -backend-config, so the module's own backend
// block stays the only one. A backend file this host wrote earlier is removed. The execution is copied
// before the argument is added, because it can be shared with the prepared configuration.
func passBackendToInit(info *schema.ConfigAndStacksInfo, workingDir string, settings map[string]any) error {
	err := configuration.WithWorkdirLock(workingDir, func() error {
		if err := RemoveHostBackend(workingDir); err != nil {
			return err
		}
		return u.WriteToFileAsHcl(filepath.Join(workingDir, backendConfigFileName), settings, backendFileMode)
	})
	if err != nil {
		return fmt.Errorf("%w: write %s: %w", errUtils.ErrBackendFileGeneration, backendConfigFileName, err)
	}
	argument := schema.ComponentExtraArguments{
		Commands: []string{"init"}, Arguments: []string{"-backend-config=" + backendConfigFileName},
	}
	execution := &schema.ComponentExecution{}
	if info.ComponentExecution != nil {
		copied := *info.ComponentExecution
		execution = &copied
	}
	for _, existing := range execution.ExtraArguments {
		if slices.Equal(existing.Commands, argument.Commands) && slices.Equal(existing.Arguments, argument.Arguments) {
			info.ComponentExecution = execution
			return nil
		}
	}
	execution.ExtraArguments = append(slices.Clone(execution.ExtraArguments), argument)
	info.ComponentExecution = execution
	return nil
}
