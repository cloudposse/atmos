package manifest

import (
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// LoadOptions holds the options a LoadOption can set on a Load call.
// Unexported: callers only ever construct one via a LoadOption function.
type LoadOptions struct {
	includeAtmosConfig *schema.AtmosConfiguration
	includeFile        string
	consumedPaths      *[]string
}

// LoadOption configures an optional Load behavior.
type LoadOption func(*LoadOptions)

// WithIncludeResolution enables !include/!include.raw tag resolution on the
// raw document tree before schema validation and decoding, so a manifest
// can use either tag anywhere a literal YAML value is otherwise accepted.
// The supplied atmosConfig provides the base path (and, for a remote
// target, any settings/credentials) !include's own local/remote dispatch
// needs; file is the nominal manifest path relative-path resolution
// anchors on -- only its directory matters, the file itself never needs to
// exist. Omitted entirely (the default for every existing Load caller),
// the raw bytes are decoded unchanged, exactly as before this option
// existed.
//
// The consumedPaths slice, when non-nil, is appended with every tag's raw
// path argument exactly as written (before resolution, and regardless of
// whether it turns out to be local or remote) -- e.g. "./lib/regions.yaml".
// A caller that also enumerates a template's own local files by the same
// relative-path convention can intersect the two sets to find which local
// files exist solely to be included, never meant to be copied into
// generated output. A remote target's raw argument is harmless to collect
// too: it simply never matches anything in a local file enumeration.
func WithIncludeResolution(atmosConfig *schema.AtmosConfiguration, file string, consumedPaths *[]string) LoadOption {
	defer perf.Track(atmosConfig, "manifest.WithIncludeResolution")()

	return func(o *LoadOptions) {
		o.includeAtmosConfig = atmosConfig
		o.includeFile = file
		o.consumedPaths = consumedPaths
	}
}

// resolveIncludeTags resolves every tag scaffold.yaml supports (see
// utils.ScaffoldTagPolicy) found anywhere in data's document tree,
// returning the re-serialized, fully resolved YAML bytes. Walks via the
// shared tag walker (pkg/utils/yaml_tag_walker.go) the stack-manifest loader
// also uses, under scaffold's own policy: !include/!include.raw and a fixed
// set of context-free tags (!env, !exec, !random, !cwd, the !git.* family,
// !literal) resolve immediately; anything else (!terraform.state, !store,
// !secret, etc. -- tags needing real stack/component/backend context a
// manifest load has no business invoking) is a hard error naming the tag,
// never silently left unresolved or deferred to a phase scaffold.yaml has
// none of.
func resolveIncludeTags(atmosConfig *schema.AtmosConfiguration, data []byte, file string, consumedPaths *[]string) ([]byte, error) {
	defer perf.Track(atmosConfig, "manifest.resolveIncludeTags")()

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanation("The document is not valid YAML").
			Err()
	}

	policy := utils.ScaffoldTagPolicy(func(path string) {
		recordConsumedPath(consumedPaths, path)
	})
	if err := utils.WalkYAMLTags(atmosConfig, &doc, file, policy); err != nil {
		return nil, err
	}

	resolved, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanationf("Failed to re-serialize `%s` after resolving its YAML tags", file).
			Err()
	}
	return resolved, nil
}

// recordConsumedPath appends val's raw path argument to consumedPaths (a
// no-op if consumedPaths is nil, or if val fails to parse -- a malformed
// argument is surfaced as a real error by ProcessIncludeTag/
// ProcessIncludeRawTag right after this call, so silently skipping it here
// is fine).
func recordConsumedPath(consumedPaths *[]string, val string) {
	if consumedPaths == nil {
		return
	}
	args, err := parser.ParseInclude(val)
	if err != nil || args.Path == "" {
		return
	}
	*consumedPaths = append(*consumedPaths, args.Path)
}
