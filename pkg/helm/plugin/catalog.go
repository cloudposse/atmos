package plugin

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// builtinCatalog maps short, friendly plugin aliases to their canonical install
// URL. Users may declare plugins by alias (e.g. "diff@v3.9.4") instead of the
// full "owner/repo" or URL form.
//
// The map is intentionally small and curated: it covers the plugins commonly
// required by helmfile-based workflows. Any plugin not listed here can still be
// installed via its "owner/repo" or full URL form.
var builtinCatalog = map[string]string{
	"diff":     "https://github.com/databus23/helm-diff",
	"secrets":  "https://github.com/jkroepke/helm-secrets",
	"git":      "https://github.com/aslafy-z/helm-git",
	"s3":       "https://github.com/hypnoglow/helm-s3",
	"unittest": "https://github.com/helm-unittest/helm-unittest",
}

// catalogURL returns the canonical install URL for a built-in alias.
func catalogURL(alias string) (string, bool) {
	url, ok := builtinCatalog[alias]
	return url, ok
}

// KnownAliases returns the sorted list of built-in plugin aliases. Used to build
// helpful error hints when an unknown alias is supplied.
func KnownAliases() []string {
	defer perf.Track(nil, "plugin.KnownAliases")()

	aliases := make([]string, 0, len(builtinCatalog))
	for alias := range builtinCatalog {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
