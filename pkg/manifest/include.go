package manifest

import (
	"strings"

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

// resolveIncludeTags resolves every !include/!include.raw tag found
// anywhere in data's document tree, returning the re-serialized, fully
// resolved YAML bytes. Deliberately narrower than the stack-manifest tag
// walker this mirrors (pkg/utils's processCustomTagsInner) -- it only
// recognizes !include/!include.raw, leaves every other tag (including
// Atmos's other YAML functions, which need real stack/backend context a
// manifest load has no business invoking) completely untouched, and never
// rejects an unrecognized tag as unsupported.
func resolveIncludeTags(atmosConfig *schema.AtmosConfiguration, data []byte, file string, consumedPaths *[]string) ([]byte, error) {
	defer perf.Track(atmosConfig, "manifest.resolveIncludeTags")()

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanation("The document is not valid YAML").
			Err()
	}

	if err := walkIncludeTags(atmosConfig, &doc, file, consumedPaths); err != nil {
		return nil, err
	}

	resolved, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanationf("Failed to re-serialize `%s` after resolving !include", file).
			Err()
	}
	return resolved, nil
}

// walkIncludeTags recurses through node's tree, resolving every
// !include/!include.raw tag it finds in place. A resolved node's own
// content is recursed into afterward (matching processCustomTagsInner's
// own order), so a transitively included file's own !include tags resolve
// too. The consumedPaths slice, when non-nil, collects each tag's raw path
// argument as encountered -- see WithIncludeResolution's doc comment.
func walkIncludeTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string, consumedPaths *[]string) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return walkIncludeTags(atmosConfig, node.Content[0], file, consumedPaths)
	}

	for _, n := range node.Content {
		tag := strings.TrimSpace(n.Tag)
		val := strings.TrimSpace(n.Value)

		switch tag {
		case utils.AtmosYamlFuncInclude:
			recordConsumedPath(consumedPaths, val)
			if err := utils.ProcessIncludeTag(atmosConfig, n, val, file); err != nil {
				return err
			}
		case utils.AtmosYamlFuncIncludeRaw:
			recordConsumedPath(consumedPaths, val)
			if err := utils.ProcessIncludeRawTag(atmosConfig, n, val, file); err != nil {
				return err
			}
		}

		if len(n.Content) > 0 {
			if err := walkIncludeTags(atmosConfig, n, file, consumedPaths); err != nil {
				return err
			}
		}
	}
	return nil
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
