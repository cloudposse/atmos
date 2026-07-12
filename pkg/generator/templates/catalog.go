package templates

import (
	_ "embed"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// fullGitSHAPattern matches a full 40-character git commit SHA. GitHub only
// allows fetching an arbitrary (non-branch/tag-tip) commit by its full SHA,
// and a shallow clone -- the default go-getter applies to git sources --
// rejects a ref that isn't a branch or tag outright, so a full-SHA ref must
// also disable the shallow clone.
var fullGitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// defaultCatalogRef returns the ref to pin unqualified catalog sources to:
// the exact commit this binary was built from (see scripts/build-atmos.sh and
// .goreleaser*.yml), so a distributable scaffold resolves against content
// that actually exists at that commit on any pushed branch, not just tagged
// releases. Binaries built without ldflags (`go run`, `go install`) fall back
// to `main`.
func defaultCatalogRef() string {
	if version.Commit != "" {
		return version.Commit
	}
	return "main"
}

// catalogData is the embedded scaffold catalog manifest.
//
//go:embed catalog.yaml
var catalogData []byte

// CatalogEntry describes a distributable scaffold template advertised by Atmos.
// Entries are resolved on demand: the list/select flows display them cheaply
// (no download) and only the selected template is fetched from its Source.
type CatalogEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Cloud       string `yaml:"cloud"`
	Tier        string `yaml:"tier"`
	Source      string `yaml:"source"`
	Version     string `yaml:"version"`
}

// catalogFile is the on-disk shape of catalog.yaml.
type catalogFile struct {
	Templates []CatalogEntry `yaml:"templates"`
}

// LoadCatalog parses the embedded scaffold catalog.
func LoadCatalog() ([]CatalogEntry, error) {
	defer perf.Track(nil, "templates.LoadCatalog")()

	var cf catalogFile
	if err := yaml.Unmarshal(catalogData, &cf); err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldCatalogLoad).
			WithCause(err).
			WithExplanation("Failed to parse the embedded scaffold catalog").
			WithHint("This indicates a corrupted Atmos binary; reinstall or rebuild with `make build`").
			WithExitCode(1).
			Err()
	}
	return cf.Templates, nil
}

// ResolvedSource returns the source to fetch this template from. When override
// is non-empty it points at a local base directory (override/<cloud>/<tier>)
// instead of the remote Source — used by CI to resolve templates from the
// working tree rather than the network. Otherwise the remote Source is pinned
// to defaultCatalogRef(); a full-SHA ref also disables the shallow clone go-getter
// would otherwise apply, since git rejects a shallow clone pinned to an
// arbitrary (non-branch/tag) commit.
func (e *CatalogEntry) ResolvedSource(override string) string {
	defer perf.Track(nil, "templates.CatalogEntry.ResolvedSource")()

	if override != "" {
		return filepath.Join(override, e.Cloud, e.Tier)
	}

	ref := defaultCatalogRef()
	src := e.Source + "?ref=" + ref
	if fullGitSHAPattern.MatchString(ref) {
		src += "&depth=0"
	}
	return src
}

// CatalogStubs returns lightweight Configuration entries (without Files) for the
// catalog, keyed by template name. They are display/selection placeholders; the
// caller hydrates the chosen template from its Source before generation.
func CatalogStubs(override string) (map[string]Configuration, error) {
	defer perf.Track(nil, "templates.CatalogStubs")()

	entries, err := LoadCatalog()
	if err != nil {
		return nil, err
	}

	stubs := make(map[string]Configuration, len(entries))
	for i := range entries {
		e := &entries[i]
		stubs[e.Name] = Configuration{
			Name:        e.Name,
			Description: e.Description,
			Version:     e.Version,
			Source:      e.ResolvedSource(override),
		}
	}
	return stubs, nil
}
