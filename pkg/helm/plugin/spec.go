package plugin

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Spec is a parsed Helm plugin declaration.
//
// It is produced from a compact string in one of the following forms:
//   - short alias:        "diff" or "diff@v3.9.4"
//   - owner/repo:         "databus23/helm-diff" or "databus23/helm-diff@v3.9.4"
//   - full URL:           "https://github.com/databus23/helm-diff@v3.9.4"
type Spec struct {
	// Name is a human-friendly label for the plugin (the alias or repo name),
	// used in log/UI messages.
	Name string
	// URL is the install URL passed to `helm plugin install`.
	URL string
	// Version is a concrete tag/ref (e.g. "v3.9.4") or empty / "latest" to
	// install the latest released version.
	Version string
	// Raw is the original, unparsed declaration.
	Raw string
}

// IsLatest reports whether the spec requests the latest version (no pinned tag).
func (s Spec) IsLatest() bool {
	defer perf.Track(nil, "plugin.Spec.IsLatest")()

	return s.Version == "" || strings.EqualFold(s.Version, "latest")
}

// candidateNames returns the plugin names that `helm plugin list` might report
// for this spec. Helm registers a plugin under the `name:` field of its
// plugin.yaml, which we cannot know without installing it. Most plugins are
// named either after their repo ("helm-git") or after the repo with the
// conventional "helm-" prefix stripped ("diff" for "helm-diff"), so we match
// against both forms.
func (s Spec) candidateNames() []string {
	base := s.Name
	stripped := strings.TrimPrefix(base, "helm-")

	seen := map[string]bool{}
	var names []string
	for _, n := range []string{base, stripped, "helm-" + stripped} {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	return names
}

// ParseSpecs parses a list of compact plugin declarations.
func ParseSpecs(raws []string) ([]Spec, error) {
	defer perf.Track(nil, "plugin.ParseSpecs")()

	specs := make([]Spec, 0, len(raws))
	for _, raw := range raws {
		spec, err := ParseSpec(raw)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// ParseSpec parses a single compact plugin declaration into a Spec.
func ParseSpec(raw string) (Spec, error) {
	defer perf.Track(nil, "plugin.ParseSpec")()

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Spec{}, errUtils.Build(errUtils.ErrInvalidHelmPluginSpec).
			WithExplanation("Empty helm plugin specification").
			WithHint("Specify a plugin as an alias (e.g. `diff@v3.9.4`), `owner/repo@version`, or a full URL").
			Err()
	}

	base, version := splitVersion(trimmed)
	if err := validateVersion(version, trimmed); err != nil {
		return Spec{}, err
	}

	spec := Spec{Version: version, Raw: trimmed}

	switch {
	case strings.Contains(base, "://"):
		spec.URL = base
		spec.Name = repoName(base)
	case isSCPURL(base):
		// SCP-style git URL, e.g. "git@github.com:owner/repo".
		spec.URL = base
		spec.Name = repoName(base)
	case strings.Contains(base, "/"):
		spec.URL = "https://github.com/" + base
		spec.Name = repoName(base)
	default:
		url, ok := catalogURL(base)
		if !ok {
			return Spec{}, errUtils.Build(errUtils.ErrInvalidHelmPluginSpec).
				WithExplanationf("Unknown helm plugin alias %q", base).
				WithHintf("Use a built-in alias (%s), an `owner/repo`, or a full URL", strings.Join(KnownAliases(), ", ")).
				Err()
		}
		spec.URL = url
		spec.Name = base
	}

	return spec, nil
}

// isSCPURL reports whether s looks like an SCP-style git URL such as
// "git@github.com:owner/repo" (contains "@" and ":" but no "://" scheme).
func isSCPURL(s string) bool {
	return strings.Contains(s, "@") && strings.Contains(s, ":")
}

// splitVersion splits a "base@version" declaration. The part after the last "@"
// is only treated as a version when it cannot be part of a URL/path (versions
// never contain "/" or ":"). This keeps SCP-style URLs like
// "git@github.com:owner/repo" intact.
func splitVersion(s string) (base, version string) {
	at := strings.LastIndex(s, "@")
	if at <= 0 {
		return s, ""
	}
	candidate := s[at+1:]
	if candidate == "" || strings.ContainsAny(candidate, "/:") {
		return s, ""
	}
	return s[:at], candidate
}

// validateVersion rejects semver constraint expressions. Helm's
// `plugin install --version` requires a concrete tag/ref, so only concrete
// versions or "latest" are accepted.
func validateVersion(version, raw string) error {
	if version == "" || strings.EqualFold(version, "latest") {
		return nil
	}
	if strings.ContainsAny(version, "^~*><= ") || strings.Contains(version, ".x") {
		return errUtils.Build(errUtils.ErrInvalidHelmPluginSpec).
			WithExplanationf("Semver constraints are not supported for helm plugins: %q", raw).
			WithHint("Pin a concrete tag (e.g. `diff@v3.9.4`) or use `latest`").
			Err()
	}
	return nil
}

// repoName extracts the final path segment of an owner/repo or URL and strips a
// trailing ".git" suffix.
func repoName(s string) string {
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
