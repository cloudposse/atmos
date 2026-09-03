package main

import (
	"fmt"
	"regexp"
)

// overrideRule is a deterministic license-URL override for a module
// go-licenses cannot resolve reliably (vanity import paths, split-module
// repos). The URL is rebuilt from the module's resolved version (via
// moduleVersion, no network access), so it is identical on every run
// regardless of whether go-licenses' own resolution succeeded.
//
// cloud.google.com/go and its submodules are deliberately not listed here:
// they follow one predictable URL shape handled generically by
// applyGoogleCloudGoOverrides instead of one entry per submodule.
type overrideRule struct {
	Module      string
	Repo        string
	RefPrefix   string
	LicensePath string
}

var repoOverrides = []overrideRule{
	{"al.essio.dev/pkg/shellescape", "github.com/alessio/shellescape", "", "LICENSE"},
	{"dario.cat/mergo", "github.com/imdario/mergo", "", "LICENSE"},
	{"inet.af/netaddr", "github.com/inetaf/netaddr", "", "LICENSE"},
	{"go4.org/intern", "github.com/go4org/intern", "", "LICENSE"},
	{"go4.org/netipx", "github.com/go4org/netipx", "", "LICENSE"},
	{"go4.org/unsafe/assume-no-moving-gc", "github.com/go4org/unsafe-assume-no-moving-gc", "", "LICENSE"},
	{"gopkg.in/ini.v1", "github.com/go-ini/ini", "", "LICENSE"},
	{"gopkg.in/evanphx/json-patch.v4", "github.com/evanphx/json-patch", "", "LICENSE"},
	{"gopkg.in/inf.v0", "github.com/go-inf/inf", "", "LICENSE"},
	{"gopkg.in/op/go-logging.v1", "github.com/op/go-logging", "", "LICENSE"},
	{"gopkg.in/warnings.v0", "github.com/go-warnings/warnings", "", "LICENSE"},
	{"gopkg.in/yaml.v2", "github.com/go-yaml/yaml", "", "LICENSE"},
	{"gopkg.in/yaml.v3", "github.com/go-yaml/yaml", "", "LICENSE"},
	{"github.com/evanphx/json-patch/v5", "github.com/evanphx/json-patch", "", "v5/LICENSE"},
	{"github.com/googleapis/gax-go/v2", "github.com/googleapis/gax-go", "", "v2/LICENSE"},
	{"github.com/blang/semver/v4", "github.com/blang/semver", "", "v4/LICENSE"},
	{"github.com/aws/aws-sdk-go-v2/internal/endpoints/v2", "github.com/aws/aws-sdk-go-v2", "internal/endpoints", "internal/endpoints/v2/LICENSE.txt"},
}

// pseudoVersionRe matches a Go pseudo-version's trailing 12-hex-char commit
// hash (e.g. v0.0.0-20240101000000-abcdef012345).
var pseudoVersionRe = regexp.MustCompile(`-([0-9a-f]{12})$`)

// gitRefFromVersion maps a module version to a ref usable in a GitHub blob
// URL: a tag (e.g. v1.0.2) is used verbatim; a pseudo-version resolves to
// its trailing commit hash, since the full pseudo-version string is not
// itself a git ref.
func gitRefFromVersion(version string) string {
	if m := pseudoVersionRe.FindStringSubmatch(version); m != nil {
		return m[1]
	}
	return version
}

// applyOverrides rewrites the URL of every entry matching a repoOverrides
// rule. Modules not present in entries, or whose version can't be
// resolved, are left untouched -- matching the previous script's silent
// skip behavior.
func applyOverrides(entries []LicenseEntry, moduleVersion func(module string) (string, error)) {
	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.Module] = i
	}

	for _, rule := range repoOverrides {
		idx, ok := index[rule.Module]
		if !ok {
			continue
		}
		version, err := moduleVersion(rule.Module)
		if err != nil || version == "" {
			continue
		}
		ref := gitRefFromVersion(version)
		if rule.RefPrefix != "" {
			ref = rule.RefPrefix + "/" + ref
		}
		entries[idx].URL = fmt.Sprintf("https://%s/blob/%s/%s", rule.Repo, ref, rule.LicensePath)
	}
}
