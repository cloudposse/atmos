package config

import (
	legacyyaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// includeResult is what the origin of one evaluated include is derived from.
type includeResult struct {
	// localPath is the local file the include resolved to, empty for a remote include.
	localPath string
	// target is the include argument as written.
	target  string
	decoded any
	// walked reports that the included content was evaluated with per-key origins already recorded.
	walked bool
	// walkedRemote is the first remote include evaluated inside this one.
	walkedRemote *includeOrigin
}

// runInclude loads an include into resolved. A raw include is read verbatim. Otherwise the included
// YAML is walked, and when origins are tracked a remote include is walked as remote content.
func (d *configYamlDecoder) runInclude(atmosConfig *schema.AtmosConfiguration, resolved *legacyyaml.Node, tag, value, sourceFile string) (bool, error) {
	switch {
	case tag == u.AtmosYamlFuncIncludeRaw:
		return false, u.ProcessIncludeRawTag(atmosConfig, resolved, value, sourceFile)
	case d.origin == nil:
		return u.ProcessIncludeTagWalked(atmosConfig, resolved, value, sourceFile, d.evaluateInclude)
	default:
		return u.ProcessIncludeTagWalkedFrom(atmosConfig, resolved, value, sourceFile, u.IncludeWalk{
			Local: d.evaluateInclude,
			Remote: func(child *legacyyaml.Node, file, source string) error {
				previous := d.remoteSource
				d.remoteSource = source
				defer func() { d.remoteSource = previous }()
				return d.evaluateInclude(child, file)
			},
			Source: d.remoteSource,
		})
	}
}

// recordIncludeOrigin records where an include's value came from, so the owning key reads the
// included file rather than the file that merely references it.
func (d *configYamlDecoder) recordIncludeOrigin(result includeResult) {
	file, remote := result.localPath, false
	if file == "" {
		file, remote = result.target, true
	}
	if d.remoteSource != "" {
		file, remote = d.remoteSource, true
	}
	if !result.walked {
		d.recordIncludedOrigins(result.decoded, file, remote)
	}
	origin := &includeOrigin{file: file, remote: remote}
	// A local include never launders remote content: when the result is not a mapping (whose
	// keys carry their own origins), the remote include nested inside it keeps its origin.
	if _, isMapping := result.decoded.(map[string]any); !origin.remote && result.walkedRemote != nil && !isMapping {
		origin = result.walkedRemote
	}
	d.noteRemote(origin)
	// Set last, so the outermost include of a nested chain is the one the owning key reads,
	// except that a remote origin is never replaced by a local one.
	d.nodeOrigin = origin
}
