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

// rootConfigSuffixes are the Terraform and OpenTofu configuration files of a root module.
var rootConfigSuffixes = []string{".tf", ".tf.json", ".tofu", ".tofu.json"}

// moduleDeclaresBackend reports whether a root module file other than the host backend declares a
// `backend` block inside a `terraform` block. A file that does not parse is skipped: Terraform reports it.
func moduleDeclaresBackend(directory string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	parser := hclparse.NewParser()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == hostBackendName || !rootConfigFile(name) {
			continue
		}
		if fileDeclaresBackend(parser, filepath.Join(directory, name)) {
			return true
		}
	}
	return false
}

func rootConfigFile(name string) bool {
	return slices.ContainsFunc(rootConfigSuffixes, func(suffix string) bool { return strings.HasSuffix(name, suffix) })
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
