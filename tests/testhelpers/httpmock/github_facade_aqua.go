package httpmock

import (
	"net/http"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AquaTool describes a fake aqua-registry package the mock serves from its aqua-registry
// endpoints ({aquaPrefix}/registry.yaml and {aquaPrefix}/pkgs/{path}/registry.yaml). Field
// names mirror the subset of aquaproj/aqua-registry YAML keys that
// pkg/toolchain/registry/aqua consumes (see aquaPackageYAML's yaml tags).
//
// Checksum verification is intentionally not modeled here: leaving it unset (the zero value)
// means the registered package has no checksum config, so the installer's checksum
// verification -- which is opt-in per package -- never activates. This keeps the mock from
// having to fake a matching checksum file, without weakening production verification: real
// registry entries that do configure a checksum are unaffected, and callers who genuinely
// need to exercise checksum verification would extend this struct rather than route around it.
type AquaTool struct {
	// Owner is repo_owner; Repo is repo_name.
	Owner string
	Repo  string
	// Name is the full registry package path (e.g. "jqlang/jq", or "kubernetes/kubernetes/kubectl"
	// for a monorepo binary). Defaults to "Owner/Repo" when empty.
	Name string
	// Type defaults to "github_release" when empty.
	Type              string
	Asset             string
	Format            string
	BinaryName        string
	SupportedEnvs     []string
	VersionConstraint string
	VersionPrefix     string
}

// path returns the registry path this tool is served under (Name if set, else "Owner/Repo").
func (t *AquaTool) path() string {
	if t.Name != "" {
		return t.Name
	}
	return t.Owner + "/" + t.Repo
}

// aquaPackageYAML is the on-the-wire YAML shape for one aqua-registry package, matching the
// field names pkg/toolchain/registry/aqua's registryPackage/indexPackage decode. It doubles as
// both a top-level registry.yaml index entry (type/repo_owner/repo_name/name are all that's
// read there) and a per-package pkgs/<path>/registry.yaml file (which also reads asset/
// format/etc.) -- unknown fields are silently ignored by whichever side isn't using them.
type aquaPackageYAML struct {
	Type              string   `yaml:"type"`
	RepoOwner         string   `yaml:"repo_owner"`
	RepoName          string   `yaml:"repo_name"`
	Name              string   `yaml:"name,omitempty"`
	Asset             string   `yaml:"asset,omitempty"`
	Format            string   `yaml:"format,omitempty"`
	BinaryName        string   `yaml:"binary_name,omitempty"`
	SupportedEnvs     []string `yaml:"supported_envs,omitempty"`
	VersionConstraint string   `yaml:"version_constraint,omitempty"`
	VersionPrefix     string   `yaml:"version_prefix,omitempty"`
}

// aquaRegistryYAML wraps a list of packages, the shape both registry.yaml (the index) and
// pkgs/<path>/registry.yaml (a single-package file) use.
type aquaRegistryYAML struct {
	Packages []aquaPackageYAML `yaml:"packages"`
}

func toAquaPackageYAML(tool *AquaTool) aquaPackageYAML {
	toolType := tool.Type
	if toolType == "" {
		toolType = "github_release"
	}
	return aquaPackageYAML{
		Type:              toolType,
		RepoOwner:         tool.Owner,
		RepoName:          tool.Repo,
		Name:              tool.Name,
		Asset:             tool.Asset,
		Format:            tool.Format,
		BinaryName:        tool.BinaryName,
		SupportedEnvs:     tool.SupportedEnvs,
		VersionConstraint: tool.VersionConstraint,
		VersionPrefix:     tool.VersionPrefix,
	}
}

// RegisterAquaTool registers a fake aqua-registry package, servable at both
// {aquaPrefix}/registry.yaml (as one index entry) and
// {aquaPrefix}/pkgs/{path}/registry.yaml (the full per-package file).
func (m *GitHubMockServer) RegisterAquaTool(tool *AquaTool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aquaTools[tool.path()] = tool
}

// tryAqua handles GET {aquaPrefix}/registry.yaml and
// GET {aquaPrefix}/pkgs/{path}/registry.yaml. Returns false (unhandled) for any other path.
func (m *GitHubMockServer) tryAqua(w http.ResponseWriter, r *http.Request) bool {
	m.mu.Lock()
	aquaPrefix := m.aquaPrefix
	m.mu.Unlock()
	prefix := aquaPrefix + "/"

	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)

	if rest == "registry.yaml" {
		m.writeAquaIndex(w)
		return true
	}

	const pkgsPrefix = "pkgs/"
	const registrySuffix = "/registry.yaml"
	if strings.HasPrefix(rest, pkgsPrefix) && strings.HasSuffix(rest, registrySuffix) {
		path := strings.TrimSuffix(strings.TrimPrefix(rest, pkgsPrefix), registrySuffix)
		m.mu.Lock()
		tool, ok := m.aquaTools[path]
		m.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return true
		}
		writeYAML(w, aquaRegistryYAML{Packages: []aquaPackageYAML{toAquaPackageYAML(tool)}})
		return true
	}

	if aquaPrefix == "" {
		// An empty aqua prefix makes prefix just "/", which every request path starts with.
		// Without this guard, an unrecognized path under an empty prefix would be claimed here
		// and answered with a 404 instead of falling through to the other route handlers
		// (release downloads, archives, the legacy raw-file fallback, etc.).
		return false
	}

	http.NotFound(w, r)
	return true
}

// writeAquaIndex serves every registered aqua tool as the top-level registry.yaml index,
// which pkg/toolchain/registry/aqua's fetchRegistryIndex uses to resolve 3-segment (monorepo)
// package paths. Tools are sorted by path so the index -- and therefore the consumer's
// last-write-wins pathIndex for monorepo siblings sharing an owner/repo -- is deterministic
// across requests instead of depending on Go's randomized map iteration order.
func (m *GitHubMockServer) writeAquaIndex(w http.ResponseWriter) {
	m.mu.Lock()
	tools := make([]*AquaTool, 0, len(m.aquaTools))
	for _, tool := range m.aquaTools {
		tools = append(tools, tool)
	}
	m.mu.Unlock()

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].path() < tools[j].path()
	})

	packages := make([]aquaPackageYAML, len(tools))
	for i, tool := range tools {
		packages[i] = toAquaPackageYAML(tool)
	}
	writeYAML(w, aquaRegistryYAML{Packages: packages})
}

// writeYAML writes v as a YAML response body with a 200 status.
func writeYAML(w http.ResponseWriter, v any) {
	data, err := yaml.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
