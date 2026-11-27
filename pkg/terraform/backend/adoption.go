package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
)

// ErrNonLocalBackend indicates that a root stores its state outside of the local backend.
var ErrNonLocalBackend = fmt.Errorf("%w: stack add currently supports a local backend", errUtils.ErrInvalidStackConfig)

// LocalWorkspaceDirectory checks that a named run can reproduce the native local
// backend from the root alone, including any previously cached init overrides.
func LocalWorkspaceDirectory(directory string, openTofu bool) (string, error) {
	defer perf.Track(nil, "backend.LocalWorkspaceDirectory")()

	layout, err := ResolveLocalState(directory, openTofu)
	return layout.Workspaces, err
}

// LocalState describes the effective native local backend for one root module.
type LocalState struct {
	DefaultPath, Workspaces string
}

// Path returns the state file used by the native local backend.
func (l LocalState) Path(workspace string) string {
	defer perf.Track(nil, "backend.LocalState.Path")()
	if workspace == "default" {
		return l.DefaultPath
	}
	return filepath.Join(l.Workspaces, workspace, "terraform.tfstate")
}

// ResolveLocalState resolves the declared and initialized native backend.
func ResolveLocalState(directory string, openTofu bool) (LocalState, error) {
	defer perf.Track(nil, "backend.ResolveLocalState")()
	entries, err := os.ReadDir(directory)
	if err != nil {
		return LocalState{}, err
	}
	settings := map[string]string{"path": "terraform.tfstate", "workspace_dir": "terraform.tfstate.d"}
	parser := hclparse.NewParser()
	for _, name := range engineConfigFiles(entries, openTofu) {
		path := filepath.Join(directory, name)
		var file *hcl.File
		var diags hcl.Diagnostics
		if strings.HasSuffix(name, ".json") {
			file, diags = parser.ParseJSONFile(path)
		} else {
			file, diags = parser.ParseHCLFile(path)
		}
		if diags.HasErrors() {
			return LocalState{}, diags
		}
		if err := localSettings(file, settings); err != nil {
			return LocalState{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := checkCachedLocalSettings(directory, settings); err != nil {
		return LocalState{}, err
	}
	resolve := func(path string) string {
		if filepath.IsAbs(path) {
			return filepath.Clean(path)
		}
		return filepath.Join(directory, path)
	}
	return LocalState{DefaultPath: resolve(settings["path"]), Workspaces: resolve(settings["workspace_dir"])}, nil
}

func localSettings(file *hcl.File, settings map[string]string) error {
	top, _, diags := file.Body.PartialContent(terraformBlockSchema)
	if diags.HasErrors() {
		return diags
	}
	schema := &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "backend", LabelNames: []string{"type"}}, {Type: "cloud"},
	}}
	for _, tf := range top.Blocks {
		content, _, diags := tf.Body.PartialContent(schema)
		if diags.HasErrors() {
			return diags
		}
		for _, block := range content.Blocks {
			if err := readLocalBlock(block, settings); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkCachedLocalSettings(directory string, settings map[string]string) error {
	data, err := os.ReadFile(filepath.Join(autoinit.DataDir(directory, nil), "terraform.tfstate"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var cache struct {
		Backend struct {
			Type   string
			Config map[string]json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return err
	}
	if cache.Backend.Type != "" && cache.Backend.Type != "local" {
		return fmt.Errorf("%w: initialized backend is %s", ErrNonLocalBackend, cache.Backend.Type)
	}
	return checkCachedPaths(cache.Backend.Config, settings)
}

func checkCachedPaths(config map[string]json.RawMessage, settings map[string]string) error {
	for key, declared := range settings {
		var value string
		if raw := config[key]; raw != nil {
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
		}
		if value != "" && filepath.Clean(value) != filepath.Clean(declared) {
			return fmt.Errorf("%w: cached backend %s differs from the root configuration; preserve the native init arguments before registration", errUtils.ErrInvalidStackConfig, key)
		}
	}
	return nil
}

func readLocalBlock(block *hcl.Block, settings map[string]string) error {
	if block.Type == "cloud" || block.Labels[0] != "local" {
		return fmt.Errorf("%w; use a path-based plan for this root", ErrNonLocalBackend)
	}
	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		return diags
	}
	for key := range settings {
		if attr := attrs[key]; attr != nil {
			value, diag := attr.Expr.Value(nil)
			if diag.HasErrors() || !value.Type().Equals(cty.String) || value.IsNull() {
				return fmt.Errorf("%w: backend %s must be a literal path to register this root", errUtils.ErrInvalidStackConfig, key)
			}
			settings[key] = value.AsString()
		}
	}
	return nil
}
