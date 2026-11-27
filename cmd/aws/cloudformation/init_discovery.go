package cloudformation

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Limit automatic inspection to templates up to 1 MiB. An explicit path bypasses discovery.
const maxDiscoveryBytes = 1 << 20

func discoverTemplates(ctx context.Context, root fs.FS) ([]string, error) {
	var paths []string
	err := fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path != "." && errors.Is(walkErr, fs.ErrPermission) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if skipTemplateDir(path) {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && isTemplateFile(path) && hasTemplateResources(root, path) {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func skipTemplateDir(path string) bool {
	if path == "." {
		return false
	}
	name := filepath.Base(path)
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor"
}

func isTemplateFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json", ".template", ".cfn":
		return true
	}
	return false
}

// Read only the top-level shape. CloudFormation intrinsic tags can remain unresolved.
func hasTemplateResources(root fs.FS, path string) bool {
	info, err := fs.Stat(root, path)
	if err != nil || info.Size() > maxDiscoveryBytes {
		return false
	}
	var template struct {
		Resources yaml.Node `yaml:"Resources"`
	}
	data, err := fs.ReadFile(root, path)
	return err == nil && yaml.Unmarshal(data, &template) == nil && template.Resources.Kind == yaml.MappingNode
}
