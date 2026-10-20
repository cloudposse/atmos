package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// FunctionInput is the common function collector contract.
type (
	FunctionInput = guard.FunctionInput
	// SourceInput is the destination collector contract. Rules with allowed_roots also receive within_allowed_roots.
	SourceInput = guard.SourceInput
)

func sourceRequest(request *Request, roots []string) (*Request, error) {
	if len(roots) == 0 {
		return request, nil
	}
	var input map[string]any
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return nil, err
	}
	path, _ := input["path"].(string)
	resolved, _ := input["path_resolved"].(bool)
	input["within_allowed_roots"] = resolved && insideRoots(path, roots)
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	copy := *request
	copy.Input = data
	return &copy, nil
}

func resolveRoots(roots []string, directory string) ([]string, error) {
	var result []string
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			return nil, fmt.Errorf("%w: empty allowed root", errUtils.ErrPolicyInvalid)
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(directory, root)
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("%w: allowed root", errUtils.ErrPolicyInvalid)
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("%w: allowed root must exist: %w", errUtils.ErrPolicyInvalid, err)
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: allowed root must be a directory", errUtils.ErrPolicyInvalid)
		}
		result = append(result, canonical)
	}
	return result, nil
}

func insideRoots(path string, roots []string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
