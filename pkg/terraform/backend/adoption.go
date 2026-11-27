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

// LocalWorkspaceDirectory checks that a named run can reproduce the native local
// backend from the root alone, including any previously cached init overrides.
func LocalWorkspaceDirectory(directory string, openTofu bool) (string, error) {
	defer perf.Track(nil, "backend.LocalWorkspaceDirectory")()

	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
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
			return "", diags
		}
		if err := localSettings(file, settings); err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := checkCachedLocalSettings(directory, settings); err != nil {
		return "", err
	}
	dir := settings["workspace_dir"]
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(directory, dir)
	}
	return dir, nil
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
		return fmt.Errorf("%w: initialized backend is %s; stack add currently supports a local backend", errUtils.ErrInvalidStackConfig, cache.Backend.Type)
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
		return fmt.Errorf("%w: stack add currently supports a local backend; use a path-based plan for this root", errUtils.ErrInvalidStackConfig)
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
