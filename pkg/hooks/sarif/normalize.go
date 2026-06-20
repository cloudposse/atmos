package sarif

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
)

const githubWorkspaceViperKey = "github-workspace"

var bindGitHubWorkspaceOnce sync.Once //nolint:gochecknoglobals // one-time viper env binding

type uriMapper struct {
	workspace  string
	sourceRoot string
	scanRoot   string
}

func normalizeArtifactURIs(data []byte, ctx *hooks.ExecContext) []byte {
	if len(data) == 0 {
		return data
	}

	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return data
	}

	mapper := newURIMapper(ctx)
	if !normalizeArtifactLocations(doc, mapper) {
		return data
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return data
	}
	return out
}

func newURIMapper(ctx *hooks.ExecContext) uriMapper {
	return uriMapper{
		workspace:  cleanAbs(githubWorkspace()),
		sourceRoot: cleanAbs(sourceComponentPath(ctx)),
		scanRoot:   cleanAbs(scanComponentPath(ctx)),
	}
}

func normalizeArtifactLocations(v any, mapper uriMapper) bool {
	switch t := v.(type) {
	case map[string]any:
		changed := normalizeArtifactLocation(t, mapper)
		for _, child := range t {
			if normalizeArtifactLocations(child, mapper) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for _, child := range t {
			if normalizeArtifactLocations(child, mapper) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func normalizeArtifactLocation(m map[string]any, mapper uriMapper) bool {
	raw, ok := m["artifactLocation"]
	if !ok {
		return false
	}
	loc, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	uri, ok := loc["uri"].(string)
	if !ok {
		return false
	}
	normalized := mapper.normalize(uri)
	if normalized == uri {
		return false
	}
	loc["uri"] = normalized
	return true
}

func (m uriMapper) normalize(uri string) string {
	if uri == "" || m.workspace == "" {
		return uri
	}
	path, ok := pathFromSARIFURI(uri)
	if !ok {
		return uri
	}
	path = filepath.Clean(filepath.FromSlash(path))

	if isAbsPath(path) {
		return m.normalizeAbs(path, uri)
	}
	return m.normalizeRel(path, uri)
}

func (m uriMapper) normalizeAbs(path, fallback string) string {
	if m.scanRoot != "" && m.sourceRoot != "" {
		if rel, ok := relUnder(m.scanRoot, path); ok {
			return m.repoRelative(filepath.Join(m.sourceRoot, rel), fallback)
		}
	}
	if rel, ok := relUnder(m.workspace, path); ok {
		return filepath.ToSlash(rel)
	}
	if m.sourceRoot != "" {
		return m.repoRelative(path, fallback)
	}
	return fallback
}

func (m uriMapper) normalizeRel(path, fallback string) string {
	if unsafeRelativePath(path) {
		return fallback
	}
	if fileExists(filepath.Join(m.workspace, path)) {
		return filepath.ToSlash(path)
	}
	if m.sourceRoot == "" {
		return fallback
	}
	return m.repoRelative(filepath.Join(m.sourceRoot, path), fallback)
}

func (m uriMapper) repoRelative(path, fallback string) string {
	rel, ok := relUnder(m.workspace, path)
	if !ok {
		return fallback
	}
	return filepath.ToSlash(rel)
}

func pathFromSARIFURI(raw string) (string, bool) {
	if isWindowsDrivePath(raw) {
		return raw, true
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
		if u.Scheme != "file" {
			return "", false
		}
		if u.Path == "" {
			return "", false
		}
		return u.Path, true
	}
	return raw, true
}

func isWindowsDrivePath(path string) bool {
	if len(path) < 3 || path[1] != ':' {
		return false
	}
	drive := path[0]
	if (drive < 'A' || drive > 'Z') && (drive < 'a' || drive > 'z') {
		return false
	}
	return path[2] == '/' || path[2] == '\\'
}

func cleanAbs(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func relUnder(root, path string) (string, bool) {
	root = cleanAbs(root)
	path = cleanAbs(path)
	rel, err := filepath.Rel(root, path)
	if err != nil || unsafeRelativePath(rel) {
		return "", false
	}
	return rel, true
}

func unsafeRelativePath(path string) bool {
	clean := filepath.Clean(path)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || isAbsPath(clean)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isAbsPath(path string) bool {
	return filepath.IsAbs(path) || filepath.VolumeName(path) != ""
}

func githubWorkspace() string {
	bindGitHubWorkspaceOnce.Do(func() {
		_ = viper.BindEnv(githubWorkspaceViperKey, "GITHUB_WORKSPACE")
	})
	return viper.GetString(githubWorkspaceViperKey)
}

func scanComponentPath(ctx *hooks.ExecContext) string {
	if ctx == nil || ctx.AtmosConfig == nil || ctx.Info == nil {
		return ""
	}
	if path, exists, err := component.BuildAndResolveWorkdirPath(ctx.AtmosConfig, ctx.Info, cfg.TerraformComponentType); err == nil && exists && path != "" {
		return path
	}
	return sourceComponentPath(ctx)
}

func sourceComponentPath(ctx *hooks.ExecContext) string {
	if ctx == nil || ctx.AtmosConfig == nil || ctx.Info == nil {
		return ""
	}
	base := ctx.AtmosConfig.TerraformDirAbsolutePath
	if base == "" {
		base = ctx.AtmosConfig.BasePathAbsolute
	}
	if base == "" {
		base = ctx.AtmosConfig.BasePath
	}
	if base == "" {
		return ""
	}
	finalComponent := ctx.Info.FinalComponent
	if finalComponent == "" {
		finalComponent = ctx.Info.ComponentFromArg
	}
	if finalComponent == "" {
		finalComponent = ctx.Info.Component
	}
	if finalComponent == "" {
		return ""
	}
	return filepath.Join(base, ctx.Info.ComponentFolderPrefix, finalComponent)
}
