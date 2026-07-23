package atmos

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VendorConfigListTool lists raw vendor manifest setting paths across the root
// manifest and every manifest it imports.
type VendorConfigListTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVendorConfigListTool creates a new vendor config list tool.
func NewVendorConfigListTool(atmosConfig *schema.AtmosConfiguration) *VendorConfigListTool {
	return &VendorConfigListTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VendorConfigListTool) Name() string {
	return "atmos_vendor_config_list"
}

// Description returns the tool description.
func (t *VendorConfigListTool) Description() string {
	return "List raw setting paths from a vendor manifest (vendor.yaml) and every manifest it imports (spec.imports), optionally filtered by a glob path pattern (e.g. 'spec.sources[*].version')."
}

// Parameters returns the tool parameters.
func (t *VendorConfigListTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "pattern",
			Description: "Optional glob pattern to filter dot-notation paths (e.g. 'spec.sources[*].version')",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "file",
			Description: "Vendor manifest file (default: ./vendor.yaml)",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// vendorConfigListEntry describes one addressable vendor manifest value, tagged
// with the source file it was found in.
type vendorConfigListEntry struct {
	File  string `json:"file"`
	Path  string `json:"path"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Execute runs the tool.
func (t *VendorConfigListTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	pattern, _ := params["pattern"].(string)

	fileParam, _ := params["file"].(string)
	rootFile, err := resolveVendorConfigFile(fileParam)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	entries, err := collectVendorConfigListEntries(rootFile, pattern)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildVendorConfigListResult(rootFile, pattern, entries), nil
}

// collectVendorConfigListEntries walks the vendor manifest's import graph
// (root file plus every manifest it imports, recursively) and returns all
// addressable dot-notation paths, tagged with their source file and filtered
// by pattern if one is given.
func collectVendorConfigListEntries(rootFile, pattern string) ([]vendorConfigListEntry, error) {
	files, err := vendoring.CollectManifestFiles(rootFile)
	if err != nil {
		return nil, err
	}

	var matcher *regexp.Regexp
	if pattern != "" {
		matcher = vendorConfigPatternRegexp(pattern)
	}

	entries := make([]vendorConfigListEntry, 0)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", atmosyaml.ErrReadFile, err)
		}

		pathEntries, err := atmosyaml.ListPathEntries(content)
		if err != nil {
			return nil, err
		}

		for _, entry := range pathEntries {
			if matcher != nil && !matcher.MatchString(entry.Path) {
				continue
			}
			entries = append(entries, vendorConfigListEntry{
				File:  file,
				Path:  entry.Path,
				Type:  entry.Type,
				Value: entry.Value,
			})
		}
	}
	return entries, nil
}

// vendorConfigPatternRegexp compiles a glob-style path pattern ('*' and '?'
// wildcards) into an anchored regexp, matching the semantics of
// pkg/list.RenderPathRowsWithPattern's path-pattern filtering.
func vendorConfigPatternRegexp(pattern string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	return regexp.MustCompile("^" + quoted + "$")
}

// buildVendorConfigListResult formats the entry listing into a tools.Result.
func buildVendorConfigListResult(rootFile, pattern string, entries []vendorConfigListEntry) *tools.Result {
	var output strings.Builder
	fmt.Fprintf(&output, "Vendor Config Entries (%d) from %s", len(entries), rootFile)
	if pattern != "" {
		fmt.Fprintf(&output, " matching %q", pattern)
	}
	output.WriteString(":\n\n")

	data := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		fmt.Fprintf(&output, "%s: %s = %s\n", entry.File, entry.Path, entry.Value)
		data = append(data, map[string]interface{}{
			"file":  entry.File,
			"path":  entry.Path,
			"type":  entry.Type,
			"value": entry.Value,
		})
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"file":    rootFile,
			"pattern": pattern,
			"entries": data,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *VendorConfigListTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VendorConfigListTool) IsRestricted() bool {
	return false
}
