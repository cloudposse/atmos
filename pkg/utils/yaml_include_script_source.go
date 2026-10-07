package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ScriptSourceKey is the internal key Atmos records next to a step's `script` when the script
	// body comes from a local !include or !include.raw file. It carries provenance (the file the
	// script was read from) through stack processing so a script's own load() calls and
	// tracebacks resolve against that file. It is visible in describe output.
	ScriptSourceKey = "script_source"

	// ScriptSourceSHA256Key is the internal key recorded next to ScriptSourceKey. It holds the
	// hex SHA-256 of the exact included content as it was inserted into the `script` value.
	// Stack inheritance deep-merges maps, so a child stack that overrides only `script` (for
	// example with an inline body) would otherwise keep the base stack's script_source. Consumers
	// honor script_source only while the script still hashes to this value (see
	// ScriptSourceMatches), which makes the provenance self-validating.
	ScriptSourceSHA256Key = "script_source_sha256"

	scriptKey      = "script"
	interpreterKey = "interpreter"
	parentDirDots  = ".."
)

// isScriptInclude reports whether the value at mapping.Content[index], tagged tag, is an
// !include/!include.raw that is the `script` of a script step.
//
// The check is deliberately cheap and the tag comparison runs first (the generic include path is
// performance critical): the value must be the value of a mapping key named `script`, and the
// mapping must also hold an `interpreter` key, which every script step requires. The caller checks
// the step type and location first so plain stack data never receives internal provenance keys.
func isScriptInclude(mapping *yaml.Node, index int, tag string) bool {
	if tag != AtmosYamlFuncInclude && tag != AtmosYamlFuncIncludeRaw {
		return false
	}
	if mapping.Kind != yaml.MappingNode || index%2 != 1 || mapping.Content[index-1].Value != scriptKey {
		return false
	}
	return mappingHasKey(mapping, interpreterKey)
}

// includeSourcePath returns the path to record as `script_source` for an !include/!include.raw
// argument, or "" when the script has no single local source file: a remote include, or one with
// a YQ expression (which changes the content, so the file is no longer the script's source).
func includeSourcePath(atmosConfig *schema.AtmosConfiguration, val, file string) string {
	parsed, err := parser.ParseInclude(val)
	if err != nil || parsed.Query != "" {
		return ""
	}
	// findLocalFile reports "" for remote includes, so they never get a script_source.
	local := findLocalFile(parsed.Path, file, atmosConfig)
	if local == "" {
		return ""
	}
	return recordedScriptSourcePath(atmosConfig, local)
}

// scriptNodeHash returns the fingerprint of a script value node once its include has been
// processed, or "" when the node is not a plain string (the include produced a mapping, a list,
// or a non-string scalar, so there is no single script body to fingerprint).
func scriptNodeHash(n *yaml.Node) string {
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!str" {
		return ""
	}
	return ScriptSourceHash(n.Value)
}

// ScriptSourceHash returns the hex SHA-256 fingerprint recorded in ScriptSourceSHA256Key for a
// script body.
func ScriptSourceHash(script string) string {
	defer perf.Track(nil, "utils.ScriptSourceHash")()

	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:])
}

// ScriptSourceMatches reports whether section carries script provenance that is still valid for
// the step's script: a recorded script_source plus a script_source_sha256 equal to the hash of
// the section's current `script`. Inherited provenance goes stale when a child stack replaces
// only the `script` of a merged step, and a stale record must be ignored. It returns the recorded
// (not yet resolved) path when the provenance is valid and "" otherwise.
func ScriptSourceMatches(section map[string]any) string {
	defer perf.Track(nil, "utils.ScriptSourceMatches")()

	recorded, _ := section[ScriptSourceKey].(string)
	recordedHash, _ := section[ScriptSourceSHA256Key].(string)
	script, isString := section[scriptKey].(string)
	if recorded == "" || recordedHash == "" || !isString || ScriptSourceHash(script) != recordedHash {
		return ""
	}
	return recorded
}

// applyScriptSource sets the sibling `script_source` and `script_source_sha256` keys on mapping.
// An include-derived value replaces one a user wrote by hand: only the loader knows which file
// the script came from. When the include has no single local source (source or hash is empty),
// any hand-written keys are removed so the step carries no provenance at all.
func applyScriptSource(mapping *yaml.Node, source, hash string) {
	if source == "" || hash == "" {
		removeMappingKeys(mapping, ScriptSourceKey, ScriptSourceSHA256Key)
		return
	}
	setMappingString(mapping, ScriptSourceKey, source)
	setMappingString(mapping, ScriptSourceSHA256Key, hash)
}

// setMappingString sets key to value in mapping, replacing an existing entry in place.
func setMappingString(mapping *yaml.Node, key, value string) {
	valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = valueNode
			return
		}
	}
	mapping.Content = append(
		mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		valueNode,
	)
}

// removeMappingKeys deletes the given keys (and their values) from mapping.
func removeMappingKeys(mapping *yaml.Node, keys ...string) {
	kept := mapping.Content[:0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if !slices.Contains(keys, mapping.Content[i].Value) {
			kept = append(kept, mapping.Content[i], mapping.Content[i+1])
		}
	}
	mapping.Content = kept
}

func mappingHasKey(mapping *yaml.Node, key string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true
		}
	}
	return false
}

// recordedScriptSourcePath returns the path stored in `script_source` for an absolute local
// file: relative to the project base path (with forward slashes, so describe output is portable
// across machines and operating systems) when the file is inside it, absolute otherwise.
func recordedScriptSourcePath(atmosConfig *schema.AtmosConfiguration, absFile string) string {
	base := scriptSourceBasePath(atmosConfig)
	if base == "" {
		return absFile
	}
	rel, err := filepath.Rel(base, absFile)
	if err != nil || rel == parentDirDots || strings.HasPrefix(rel, parentDirDots+string(filepath.Separator)) {
		return absFile
	}
	return filepath.ToSlash(rel)
}

// ResolveRecordedScriptSource turns a recorded `script_source` value back into an absolute
// path: a relative value is joined with the project base path, an absolute one is returned as is.
// It returns "" for an empty value.
func ResolveRecordedScriptSource(atmosConfig *schema.AtmosConfiguration, recorded string) string {
	defer perf.Track(atmosConfig, "utils.ResolveRecordedScriptSource")()

	if recorded == "" {
		return ""
	}
	path := filepath.FromSlash(recorded)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := scriptSourceBasePath(atmosConfig)
	if base == "" {
		if abs, err := filepath.Abs(path); err == nil {
			return abs
		}
		return path
	}
	return filepath.Clean(filepath.Join(base, path))
}

// scriptSourceBasePath returns the absolute project base path ("" when it cannot be determined).
func scriptSourceBasePath(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig == nil {
		return ""
	}
	base := atmosConfig.BasePathAbsolute
	if base == "" {
		base = atmosConfig.BasePath
	}
	if base == "" {
		return ""
	}
	if abs, err := filepath.Abs(base); err == nil {
		return abs
	}
	return base
}

// prepareScriptSource captures an include's path before the tag walker replaces
// its node, then records the evaluated content fingerprint after the walk.
// Only the stack policy installs this callback; scaffold manifests are unaffected.
func prepareScriptSource(ctx TagContext, node *yaml.Node) func() {
	if ctx.dataSection || !isScriptSourceStep(ctx.parent, node) {
		return nil
	}
	for i, value := range node.Content {
		tag := strings.TrimSpace(value.Tag)
		if !isScriptInclude(node, i, tag) {
			continue
		}
		source := includeSourcePath(ctx.AtmosConfig, strings.TrimSpace(value.Value), ctx.File)
		return func() { applyScriptSource(node, source, scriptNodeHash(value)) }
	}
	return nil
}

// isScriptSourceStep accepts a typed script step or the payload of a typed script hook.
func isScriptSourceStep(parent, node *yaml.Node) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	if stepType := scriptSourceMappingValue(node, "type"); stepType != nil && stepType.Value == "script" {
		return true
	}
	stepType := scriptSourceMappingValue(parent, "type")
	return stepType != nil && stepType.Value == "script" && scriptSourceMappingValue(parent, "with") == node
}

func scriptSourceMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// isScriptSourceDataValue identifies plain data by its mapping key before tag handlers rewrite it.
func isScriptSourceDataValue(parent *yaml.Node, index int) bool {
	if parent.Kind != yaml.MappingNode || index%2 != 1 {
		return false
	}
	switch parent.Content[index-1].Value {
	case "vars", "settings", "env", "metadata", "mocks", "locals", "secrets", "backend", "providers":
		return true
	}
	return false
}
