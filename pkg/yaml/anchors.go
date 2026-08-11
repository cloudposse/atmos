package yaml

import (
	"fmt"
	"sort"
	"strings"

	goyaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// anchorInfo captures, for a single YAML anchor, a canonical serialization of
// its anchored subtree and how many aliases reference it. This is enough to
// detect both anchor-structure changes and silent value mutations of a shared
// anchor.
type anchorInfo struct {
	content    string
	aliasCount int
}

// guardAnchors enforces the strict anchor/alias contract: an edit must not
// alter or expand any anchor or alias. It compares the anchor topology of the
// document before and after an edit and returns ErrYAMLAnchorAltered if:
//
//   - an anchor was added, removed, or renamed;
//   - the number of aliases referencing an anchor changed (an alias was
//     flattened to a literal, or a new alias appeared); or
//   - the content of an anchor that is referenced by at least one alias
//     changed (a "shared" value was mutated, which silently affects every
//     location that aliases it — exactly the case yqlib performs without
//     complaint when you assign through an alias).
//
// Editing a value that participates in no anchor/alias relationship always
// passes.
func guardAnchors(before, after []byte) error {
	defer perf.Track(nil, "yaml.guardAnchors")()

	beforeAnchors, err := collectAnchors(before)
	if err != nil {
		return err
	}
	afterAnchors, err := collectAnchors(after)
	if err != nil {
		return err
	}

	if diff := compareAnchorSets(beforeAnchors, afterAnchors); diff != "" {
		return fmt.Errorf("%w: %s", ErrYAMLAnchorAltered, diff)
	}
	return nil
}

// compareAnchorSets returns a human-readable description of the first detected
// violation, or "" if the two anchor maps are compatible under the strict
// contract.
func compareAnchorSets(before, after map[string]anchorInfo) string {
	names := make(map[string]struct{}, len(before))
	for name := range before {
		names[name] = struct{}{}
	}
	for name := range after {
		names[name] = struct{}{}
	}

	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	for _, name := range ordered {
		if msg := anchorViolation(name, before, after); msg != "" {
			return msg
		}
	}
	return ""
}

// anchorViolation returns a description of how anchor `name` changed between the
// before/after maps in a way the strict contract forbids, or "" if it is fine.
func anchorViolation(name string, before, after map[string]anchorInfo) string {
	b, hadBefore := before[name]
	a, hasAfter := after[name]

	switch {
	case hadBefore && !hasAfter:
		return fmt.Sprintf("anchor &%s was removed or expanded by the edit", name)
	case !hadBefore && hasAfter:
		return fmt.Sprintf("anchor &%s was introduced by the edit", name)
	case b.aliasCount != a.aliasCount:
		return fmt.Sprintf("anchor &%s alias references changed from %d to %d (an alias was flattened or added)",
			name, b.aliasCount, a.aliasCount)
	case b.aliasCount > 0 && b.content != a.content:
		return fmt.Sprintf("anchor &%s is shared by %d alias(es) and its value would change; "+
			"edit the anchor definition explicitly or restructure to avoid mutating shared data",
			name, b.aliasCount)
	}
	return ""
}

// collectAnchors walks a YAML document and records, per anchor name, a
// canonical serialization of its anchored node plus the count of aliases that
// reference it.
func collectAnchors(content []byte) (map[string]anchorInfo, error) {
	var root goyaml.Node
	if err := goyaml.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseYAML, err)
	}

	anchors := make(map[string]anchorInfo)
	aliasCounts := make(map[string]int)
	if dup := walkAnchorNodes(&root, anchors, aliasCounts); dup != "" {
		// A redefined anchor name means aliases before and after the second
		// definition resolve to different values, so the per-name comparison
		// below cannot detect a silent shared-value mutation. Refuse to edit.
		return nil, fmt.Errorf("%w: anchor &%s is defined more than once; rename the duplicate anchors before editing", ErrYAMLDuplicateAnchor, dup)
	}

	// Fold alias counts into the anchor records.
	for name, count := range aliasCounts {
		info := anchors[name]
		info.aliasCount = count
		anchors[name] = info
	}
	return anchors, nil
}

// walkAnchorNodes recursively records anchored nodes and alias references. It
// returns the name of the first anchor defined more than once, or "".
func walkAnchorNodes(node *goyaml.Node, anchors map[string]anchorInfo, aliasCounts map[string]int) string {
	if node == nil {
		return ""
	}

	if node.Anchor != "" {
		if _, exists := anchors[node.Anchor]; exists {
			return node.Anchor
		}
		anchors[node.Anchor] = anchorInfo{content: serializeNode(node)}
	}
	if node.Kind == goyaml.AliasNode {
		// yaml.v3 stores the referenced anchor name in Value for alias nodes.
		aliasCounts[node.Value]++
	}

	for _, child := range node.Content {
		if dup := walkAnchorNodes(child, anchors, aliasCounts); dup != "" {
			return dup
		}
	}
	return ""
}

// serializeNode renders a node's anchored subtree to a stable string for value
// comparison. The anchor name itself is stripped so that comparisons reflect
// only the anchored value, not the (already separately compared) anchor name.
func serializeNode(node *goyaml.Node) string {
	clone := *node
	clone.Anchor = ""
	out, err := goyaml.Marshal(&clone)
	if err != nil {
		// Fall back to a structural fingerprint; marshal of a plain node
		// essentially never fails, so this is defensive only.
		return fmt.Sprintf("kind=%d tag=%s value=%q", node.Kind, node.Tag, node.Value)
	}
	return strings.TrimRight(string(out), "\n")
}
