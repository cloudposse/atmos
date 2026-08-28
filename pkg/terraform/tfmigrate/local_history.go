package tfmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultConfigFile     = ".tfmigrate.hcl"
	historyDirPermissions = 0o755
)

// EnsureLocalHistoryDir creates the parent directory of a tfmigrate local
// history file so first runs don't fail with "no such file or directory":
// tfmigrate's local storage writes the history file without creating missing
// directories, and a failed history save after a successful state push leaves
// the migration applied but unrecorded, wedging subsequent runs.
//
// The tfmigrate config is parsed best-effort. Any read, parse, or evaluation
// problem leaves the filesystem untouched and returns nil, keeping tfmigrate
// itself the source of truth for config errors. Only a failed directory
// creation is returned as an error.
func EnsureLocalHistoryDir(componentDir, configPath string, env []string) error {
	defer perf.Track(nil, "tfmigrate.EnsureLocalHistoryDir")()

	historyPath := localHistoryPath(componentDir, configPath, env)
	if historyPath == "" {
		return nil
	}
	if !filepath.IsAbs(historyPath) {
		historyPath = filepath.Join(componentDir, historyPath)
	}
	dir := filepath.Dir(historyPath)
	if err := os.MkdirAll(dir, historyDirPermissions); err != nil {
		return fmt.Errorf("failed to create tfmigrate local history directory %q: %w", dir, err)
	}
	return nil
}

// localHistoryPath extracts the `tfmigrate { history { storage "local" { path } } }`
// value from the tfmigrate config file, evaluating ${env.*} references against
// the supplied environment. Returns "" when the config is missing, uses a
// non-local storage, or cannot be evaluated.
func localHistoryPath(componentDir, configPath string, env []string) string {
	body := parseTfmigrateConfigBody(componentDir, configPath)
	if body == nil {
		return ""
	}

	pathExpr := findLocalStoragePathExpr(body)
	if pathExpr == nil {
		return ""
	}
	value, diags := pathExpr.Value(&hcl.EvalContext{
		Variables: map[string]cty.Value{"env": envObject(env)},
	})
	if diags.HasErrors() || value.Type() != cty.String || value.IsNull() {
		return ""
	}
	return value.AsString()
}

// parseTfmigrateConfigBody reads and parses the tfmigrate config file, resolving
// the path relative to the component directory. Returns nil on any read or
// parse problem (best-effort semantics).
func parseTfmigrateConfigBody(componentDir, configPath string) *hclsyntax.Body {
	if configPath == "" {
		configPath = defaultConfigFile
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(componentDir, configPath)
	}
	src, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	file, diags := hclparse.NewParser().ParseHCL(src, configPath)
	if diags.HasErrors() || file == nil {
		return nil
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}
	return body
}

// findLocalStoragePathExpr walks tfmigrate > history > storage "local" > path.
func findLocalStoragePathExpr(body *hclsyntax.Body) hclsyntax.Expression {
	for _, tfmigrateBlock := range blocksOfType(body, "tfmigrate") {
		for _, historyBlock := range blocksOfType(tfmigrateBlock.Body, "history") {
			for _, storageBlock := range blocksOfType(historyBlock.Body, "storage") {
				if len(storageBlock.Labels) == 0 || storageBlock.Labels[0] != "local" {
					continue
				}
				if attr, ok := storageBlock.Body.Attributes["path"]; ok {
					return attr.Expr
				}
			}
		}
	}
	return nil
}

func blocksOfType(body *hclsyntax.Body, blockType string) []*hclsyntax.Block {
	var blocks []*hclsyntax.Block
	for _, block := range body.Blocks {
		if block.Type == blockType {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// envObject merges the process environment with the supplied subprocess
// environment (the latter wins) into a cty object for ${env.*} evaluation,
// mirroring what tfmigrate itself sees.
func envObject(env []string) cty.Value {
	values := map[string]cty.Value{}
	for _, entry := range append(os.Environ(), env...) {
		key, value, found := strings.Cut(entry, "=")
		if found && key != "" {
			values[key] = cty.StringVal(value)
		}
	}
	if len(values) == 0 {
		return cty.EmptyObjectVal
	}
	return cty.ObjectVal(values)
}
