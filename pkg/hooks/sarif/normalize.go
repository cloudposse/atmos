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

var bindGitHubWorkspaceOnce sync.Once //nolint:gochecknoglobals // one-time viper env binding.

type uriMapper struct {
	workspace  string
	baseRoot   string
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
		baseRoot:   cleanAbs(atmosBasePath(ctx)),
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

// normalizeRel maps a relative SARIF path to a repo-relative path. Scanners
// disagree on what a relative path is anchored to: kics and Checkov emit paths
// relative to the Atmos working directory (e.g. components/terraform/<c>/x.tf),
// while others emit just the file name relative to the component dir. Resolve by
// trying each candidate base and using the first that points at a real file, so
// a base that already contains the component prefix is not prepended again
// (which previously produced doubled paths like
// components/terraform/<c>/components/terraform/<c>/x.tf that GitHub Code
// Scanning could not anchor).
func (m uriMapper) normalizeRel(path, fallback string) string {
	if unsafeRelativePath(path) {
		return fallback
	}
	// A path under the provisioned scan dir maps back to the committed source
	// component dir (mirrors normalizeAbs) so per-stack workdirs resolve to
	// files that actually exist in the repository.
	if m.scanRoot != "" && m.sourceRoot != "" && fileExists(filepath.Join(m.scanRoot, path)) {
		return m.repoRelative(filepath.Join(m.sourceRoot, path), fallback)
	}
	for _, base := range m.relativeBases() {
		abs := filepath.Join(base, path)
		if !fileExists(abs) {
			continue
		}
		if rel, ok := relUnder(m.workspace, abs); ok {
			return filepath.ToSlash(rel)
		}
	}
	// Nothing resolved on disk; preserve prior best-effort behavior.
	if fileExists(filepath.Join(m.workspace, path)) {
		return filepath.ToSlash(path)
	}
	if m.sourceRoot == "" {
		return fallback
	}
	return m.repoRelative(filepath.Join(m.sourceRoot, path), fallback)
}

// relativeBases lists the directories a relative SARIF path may be anchored to,
// deduped and non-empty.
func (m uriMapper) relativeBases() []string {
	candidates := []string{m.workspace, m.baseRoot, m.scanRoot, m.sourceRoot}
	seen := make(map[string]bool, len(candidates))
	bases := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		bases = append(bases, c)
	}
	return bases
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

const (
	githubServerURLViperKey  = "github-server-url"
	githubRepositoryViperKey = "github-repository"
	githubSHAViperKey        = "github-sha"
	defaultGitHubServerURL   = "https://github.com"
)

var bindGitHubBlobEnvOnce sync.Once //nolint:gochecknoglobals // one-time viper env binding.

// githubBlobBaseURL returns "<server>/<owner>/<repo>/blob/<sha>" when running in GitHub
// Actions (GITHUB_REPOSITORY and GITHUB_SHA both set), or "" otherwise. A finding's file
// is meaningless as a link outside the machine that produced it (a local filesystem
// path), but inside GitHub Actions it can point at the exact file on GitHub instead.
func githubBlobBaseURL() string {
	bindGitHubBlobEnvOnce.Do(func() {
		_ = viper.BindEnv(githubServerURLViperKey, "GITHUB_SERVER_URL")
		_ = viper.BindEnv(githubRepositoryViperKey, "GITHUB_REPOSITORY")
		_ = viper.BindEnv(githubSHAViperKey, "GITHUB_SHA")
	})
	repo := viper.GetString(githubRepositoryViperKey)
	sha := viper.GetString(githubSHAViperKey)
	if repo == "" || sha == "" {
		return ""
	}
	serverURL := viper.GetString(githubServerURLViperKey)
	if serverURL == "" {
		serverURL = defaultGitHubServerURL
	}
	return serverURL + "/" + repo + "/blob/" + sha
}

// atmosBasePath returns the absolute Atmos base path (the working directory),
// which is the root that kics and Checkov anchor their relative SARIF paths to.
func atmosBasePath(ctx *hooks.ExecContext) string {
	if ctx == nil || ctx.AtmosConfig == nil {
		return ""
	}
	if ctx.AtmosConfig.BasePathAbsolute != "" {
		return ctx.AtmosConfig.BasePathAbsolute
	}
	return ctx.AtmosConfig.BasePath
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
