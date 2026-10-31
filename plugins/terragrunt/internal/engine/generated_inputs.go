package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// unitLockFile hands the host the lock file kept beside terragrunt.hcl, so the provider versions
// and hashes it pins take effect in the working directory. The host rewrites it only when this
// copy changes, so what `init` records in the working directory is not undone on every run.
// Atmos does not write lock file changes back to the unit directory.
func unitLockFile(result *model.Resolution) error {
	if _, exists := result.GeneratedPolicies[lockFileName]; exists {
		// An explicit generate block for the lock file wins over the copy.
		return nil
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(result.Path), lockFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the lock file beside %s: %w", filepath.Base(result.Path), err)
	}
	result.Generated[lockFileName] = string(content)
	result.GeneratedPolicies[lockFileName] = "overwrite"
	return nil
}

// declaresBackend reports whether generated Terraform configuration holds a backend or cloud block.
// Content that cannot be parsed is not a backend: Terraform reports its own syntax error later.
func declaresBackend(path, content string) bool {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tf.json"):
		return jsonDeclaresBackend([]byte(content))
	case strings.HasSuffix(lower, ".tf"):
		return hclDeclaresBackend([]byte(content), path)
	default:
		return false
	}
}

func hclDeclaresBackend(content []byte, path string) bool {
	file, diagnostics := hclsyntax.ParseConfig(content, path, hcl.InitialPos)
	if diagnostics.HasErrors() {
		return false
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return false
	}
	for _, block := range body.Blocks {
		if block.Type != "terraform" {
			continue
		}
		for _, nested := range block.Body.Blocks {
			if nested.Type == "backend" || nested.Type == "cloud" {
				return true
			}
		}
	}
	return false
}

func jsonDeclaresBackend(content []byte) bool {
	var document struct {
		Terraform json.RawMessage `json:"terraform"`
	}
	if json.Unmarshal(content, &document) != nil || len(document.Terraform) == 0 {
		return false
	}
	// "terraform" is an object, or a list of objects in the alternate JSON syntax.
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(document.Terraform, &blocks) != nil {
		var single map[string]json.RawMessage
		if json.Unmarshal(document.Terraform, &single) != nil {
			return false
		}
		blocks = []map[string]json.RawMessage{single}
	}
	for _, block := range blocks {
		if _, ok := block["backend"]; ok {
			return true
		}
		if _, ok := block["cloud"]; ok {
			return true
		}
	}
	return false
}
