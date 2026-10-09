package dependencies

import (
	"slices"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	systemVersion     = "system"
	refVersionPrefix  = "ref:"
	pathVersionPrefix = "path:"

	// The logKeyTool constant is the structured log and error context key naming a tool.
	logKeyTool = "tool"
)

// constraintOperatorTokens are bare comparison operators. A manifest line such as
// `terraform ~> 1.9.0` (with a space) parses into the version token `~>`.
var constraintOperatorTokens = map[string]bool{
	"~>": true, ">=": true, "<=": true, ">": true, "<": true,
	"=": true, "^": true, "~": true, "!=": true,
}

// defaultGroup is the set of manifest keys that identify one tool.
type defaultGroup struct {
	identity string
	// keys are the manifest keys, sorted. The first is the representative key
	// handed to the installer.
	keys    []string
	version string
}

func (g *defaultGroup) key() string { return g.keys[0] }

// sortedKeys returns the keys of m in sorted order for deterministic output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// usableDefaults drops manifest entries that cannot be applied as project
// defaults: `system`, `ref:` and `path:` versions, and bare operator tokens.
// Dropped entries are logged and never abort a run.
func usableDefaults(manifest map[string]string) map[string]string {
	usable := make(map[string]string, len(manifest))
	for _, tool := range sortedKeys(manifest) {
		version := strings.TrimSpace(manifest[tool])
		switch {
		case version == systemVersion:
			log.Debug("Skipping .tool-versions entry: version is system", logKeyTool, tool)
		case strings.HasPrefix(version, refVersionPrefix), strings.HasPrefix(version, pathVersionPrefix):
			log.Debug("Skipping .tool-versions entry: version is not an installable release", logKeyTool, tool, "version", version)
		case constraintOperatorTokens[version]:
			log.Warn("Skipping .tool-versions entry: the version is a bare constraint operator. Remove the space between the operator and the version (for example `~>1.9.0`)",
				logKeyTool, tool, "version", version)
		default:
			usable[tool] = version
		}
	}
	return usable
}

// groupDefaults collapses manifest entries that identify the same tool. Entries
// whose tool cannot be resolved, or that an explicit dependency overrides, are
// skipped. The same tool pinned to two different versions is an error.
func groupDefaults(usable map[string]string, overridden map[string]bool, ids *toolIdentity) ([]defaultGroup, error) {
	byIdentity := make(map[string]int, len(usable))
	var groups []defaultGroup
	for _, tool := range sortedKeys(usable) {
		id, err := ids.identity(tool)
		if err != nil {
			log.Debug("Skipping .tool-versions entry: tool cannot be resolved", logKeyTool, tool, "error", err)
			continue
		}
		if overridden[id] {
			log.Debug("Skipping .tool-versions entry: overridden by an explicit dependency", logKeyTool, tool)
			continue
		}
		version := usable[tool]
		idx, seen := byIdentity[id]
		if !seen {
			byIdentity[id] = len(groups)
			groups = append(groups, defaultGroup{identity: id, keys: []string{tool}, version: version})
			continue
		}
		group := &groups[idx]
		if group.version != version {
			return nil, versionConflictError(id, group.keys[0], group.version, tool, version)
		}
		group.keys = append(group.keys, tool)
		slices.Sort(group.keys)
	}
	return groups, nil
}

// versionConflictError reports two manifest entries that name the same tool with
// different versions.
func versionConflictError(identity, firstKey, firstVersion, secondKey, secondVersion string) error {
	return errUtils.Build(errUtils.ErrToolVersionsConflict).
		WithExplanationf("The .tool-versions entries `%s %s` and `%s %s` both identify %s.", firstKey, firstVersion, secondKey, secondVersion, identity).
		WithHintf("Keep a single entry for %s in .tool-versions, or pin the same version in both `%s` and `%s`", identity, firstKey, secondKey).
		WithContext(logKeyTool, identity).
		WithContext("entries", firstKey+", "+secondKey).
		Err()
}
