package merge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Constants for YAML merging.
const (
	maxChangePercentage = 100 // Maximum change percentage when parsing fails.
)

// YAMLMerger handles 3-way merging of YAML files with structure awareness.
// It preserves comments, anchors, and performs intelligent key-level merging.
//
// Why not use pkg/merge (mergego)?
// The pkg/merge package uses dario.cat/mergo for runtime map[string]any merging during
// stack configuration processing. This YAMLMerger serves a fundamentally different purpose:
//
//   - pkg/merge: Merges already-parsed data structures (map[string]any) for stack inheritance
//   - pkg/generator/merge: Performs git-style 3-way merges for template updates with conflict detection
//
// Key differences:
//  1. Level of operation: mergo works on Go data structures; this works on YAML nodes
//  2. Merge strategy: mergo does 2-way merges; this does 3-way merges (base, ours, theirs)
//  3. Conflict detection: mergo overwrites; this detects and reports conflicts
//  4. Preservation: mergo doesn't preserve YAML formatting; this preserves comments and anchors
//  5. Use case: mergo for config inheritance; this for updating user-modified files from templates
//
// Example: When a user runs "atmos init --update", this merger compares:
//   - base: the original template file (from git history)
//   - ours: the user's modified version (current working directory)
//   - theirs: the new template version (from updated template)
//
// It intelligently merges changes, preserving user customizations while incorporating template updates.
type YAMLMerger struct {
	thresholdPercent int              // Percentage threshold (0-100) for change detection
	conflictStrategy ConflictStrategy // How to resolve a real ours/theirs divergence
}

// NewYAMLMerger creates a new YAML merger with the specified percentage threshold.
func NewYAMLMerger(thresholdPercent int) *YAMLMerger {
	defer perf.Track(nil, "merge.NewYAMLMerger")()

	return &YAMLMerger{
		thresholdPercent: thresholdPercent,
	}
}

// SetConflictStrategy sets how a real ours/theirs divergence is resolved.
// The zero value (ConflictStrategyManual) is today's existing behavior.
func (m *YAMLMerger) SetConflictStrategy(strategy ConflictStrategy) {
	defer perf.Track(nil, "merge.YAMLMerger.SetConflictStrategy")()

	m.conflictStrategy = strategy
}

// pickConflictValue resolves a real ours/theirs divergence per the configured
// conflict strategy. Manual (default) still records the conflict via
// conflicts.addConflict — MergeResult.HasConflicts then aborts the write in
// engine.Processor.mergeFile, which is today's existing "surface, don't
// silently pick a side" behavior. Ours/theirs pick a side and deliberately do
// not record a conflict, so the write proceeds.
func (m *YAMLMerger) pickConflictValue(ours, theirs *yaml.Node, path string, conflicts *conflictTracker) *yaml.Node {
	switch m.conflictStrategy {
	case ConflictStrategyTheirs:
		return theirs
	case ConflictStrategyOurs:
		return ours
	default:
		conflicts.addConflict(path)
		return ours
	}
}

// Merge performs a 3-way merge of YAML content with structure awareness.
// Parameters:
//   - base: The original YAML content (common ancestor)
//   - ours: The user's YAML version (with their changes)
//   - theirs: The template's YAML version (with template updates)
//
// Returns the merged YAML content or an error if conflicts exceed threshold.
//
//nolint:revive,funlen // function-length: rich error handling with static errors requires additional lines
func (m *YAMLMerger) Merge(base, ours, theirs string) (*MergeResult, error) {
	defer perf.Track(nil, "merge.YAMLMerger.Merge")()

	// Parse all three YAML documents. A stream may contain more than one
	// `---`-separated document; decodeAllDocuments preserves every one of
	// them instead of silently keeping only the first (yaml.Unmarshal would
	// discard the rest of the stream).
	baseDocs, err := decodeAllDocuments(base, "base")
	if err != nil {
		return nil, err
	}

	oursDocs, err := decodeAllDocuments(ours, "user's")
	if err != nil {
		return nil, err
	}

	theirsDocs, err := decodeAllDocuments(theirs, "template")
	if err != nil {
		return nil, err
	}

	// Perform structure-aware merge, document by document.
	conflicts := &conflictTracker{conflicts: make([]string, 0)}
	mergedDocs, err := m.mergeDocumentStreams(baseDocs, oursDocs, theirsDocs, conflicts)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithCause(err).
			WithExplanation("Failed to merge YAML structures").
			WithHint("Check for incompatible changes between versions").
			Err()
	}

	// Check if conflicts exceed threshold
	if len(conflicts.conflicts) > 0 && m.thresholdPercent > 0 {
		changePercentage := m.calculateYAMLChangePercentage(base, ours, theirs, len(conflicts.conflicts))
		if changePercentage > m.thresholdPercent {
			return nil, errUtils.Build(errUtils.ErrMergeThresholdExceeded).
				WithExplanationf("Too many YAML conflicts detected (%d%% changes, threshold: %d%%). %d conflicts found", changePercentage, m.thresholdPercent, len(conflicts.conflicts)).
				WithHint("Use --force to overwrite or manually merge").
				Err()
		}
	}

	// Marshal back to YAML, preserving formatting. Encoding each document in
	// turn on the same encoder emits the `---` document separators between
	// them automatically, so a multi-document stream round-trips as one.
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	for _, doc := range mergedDocs {
		if err := encoder.Encode(doc); err != nil {
			return nil, errUtils.Build(errUtils.ErrEncode).
				WithCause(err).
				WithExplanation("Failed to encode merged YAML").
				WithHint("This may indicate corrupted merge output").
				Err()
		}
	}

	if err := encoder.Close(); err != nil {
		return nil, errUtils.Build(errUtils.ErrEncode).
			WithCause(err).
			WithExplanation("Failed to close YAML encoder").
			Err()
	}

	return &MergeResult{
		Content:       buf.String(),
		HasConflicts:  len(conflicts.conflicts) > 0,
		ConflictCount: len(conflicts.conflicts),
	}, nil
}

// decodeAllDocuments decodes every `---`-separated document in a YAML stream,
// returning one *yaml.Node (Kind: DocumentNode) per document. A single-document
// stream returns a single-element slice, so callers written for that case still
// work unchanged when indexed at position 0.
func decodeAllDocuments(content, label string) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(strings.NewReader(content))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errUtils.Build(errUtils.ErrScaffoldParseYAML).
				WithCause(err).
				WithExplanationf("Failed to parse %s YAML", label).
				WithHint("Check YAML syntax").
				Err()
		}
		d := doc
		docs = append(docs, &d)
	}
	return docs, nil
}

// docAt returns the document at index i, or nil if the stream is shorter than i+1.
func docAt(docs []*yaml.Node, i int) *yaml.Node {
	if i < len(docs) {
		return docs[i]
	}
	return nil
}

// mergeDocumentStreams merges base/ours/theirs document streams pairwise, by
// document index. A document present on only one side (stream length
// mismatch) is kept as-is; a document the template changed that the user's
// stream no longer has is recorded as a conflict rather than silently dropped.
//
//nolint:revive // cyclomatic: pairwise document-presence handling requires this many branches
func (m *YAMLMerger) mergeDocumentStreams(baseDocs, oursDocs, theirsDocs []*yaml.Node, conflicts *conflictTracker) ([]*yaml.Node, error) {
	maxLen := len(baseDocs)
	if len(oursDocs) > maxLen {
		maxLen = len(oursDocs)
	}
	if len(theirsDocs) > maxLen {
		maxLen = len(theirsDocs)
	}

	result := make([]*yaml.Node, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		base, ours, theirs := docAt(baseDocs, i), docAt(oursDocs, i), docAt(theirsDocs, i)
		docPath := fmt.Sprintf("documents[%d]", i)

		switch {
		case ours == nil && theirs == nil:
			continue // Neither side has this document.
		case ours == nil:
			if base != nil && !nodesEqual(base, theirs) {
				conflicts.addConflict(docPath) // Template changed a document the user's stream dropped.
			}
			result = append(result, theirs)
			continue
		case theirs == nil:
			result = append(result, ours) // Template no longer has it; keep the user's document.
			continue
		}

		if base == nil {
			base = createEmptyNodeOfKind(ours)
		}
		merged, err := m.mergeNodes(base, ours, theirs, docPath, conflicts)
		if err != nil {
			return nil, err
		}
		result = append(result, merged)
	}
	return result, nil
}

// conflictTracker keeps track of conflicts during merge.
type conflictTracker struct {
	conflicts []string
}

// addConflict records a conflict at the given path.
func (c *conflictTracker) addConflict(path string) {
	c.conflicts = append(c.conflicts, path)
}

// mergeNodes recursively merges YAML nodes.
//
//nolint:revive // cyclomatic: 3-way merge requires handling multiple node states
func (m *YAMLMerger) mergeNodes(base, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	// If all three are identical, return any of them
	if nodesEqual(base, ours) && nodesEqual(base, theirs) {
		return ours, nil
	}

	// If ours and base are same, use theirs (template updated)
	if nodesEqual(base, ours) {
		return theirs, nil
	}

	// If theirs and base are same, use ours (user updated)
	if nodesEqual(base, theirs) {
		return ours, nil
	}

	// If ours and theirs are same, use either (both made same change)
	if nodesEqual(ours, theirs) {
		return ours, nil
	}

	// Guard against kind divergence: if ours or theirs changed to a different
	// node kind than base, dispatching on base.Kind would call the wrong merge
	// helper and produce a node of the wrong shape (e.g., calling mergeScalars
	// when ours is a MappingNode).  Record a conflict and preserve the user's
	// version whenever kinds diverge.
	if ours.Kind != base.Kind || theirs.Kind != base.Kind {
		return m.pickConflictValue(ours, theirs, path, conflicts), nil
	}

	// Handle based on node type.
	switch base.Kind {
	case yaml.DocumentNode:
		return m.mergeDocuments(base, ours, theirs, path, conflicts)
	case yaml.MappingNode:
		return m.mergeMappings(base, ours, theirs, path, conflicts)
	case yaml.SequenceNode:
		return m.mergeSequences(base, ours, theirs, path, conflicts)
	case yaml.ScalarNode:
		return m.mergeScalars(base, ours, theirs, path, conflicts)
	default:
		// For other node types, prefer ours if different.
		return ours, nil
	}
}

// mergeDocuments merges document nodes.
func (m *YAMLMerger) mergeDocuments(base, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	result := &yaml.Node{
		Kind: yaml.DocumentNode,
	}

	// Merge the content nodes.
	switch {
	case len(base.Content) > 0 && len(ours.Content) > 0 && len(theirs.Content) > 0:
		merged, err := m.mergeNodes(base.Content[0], ours.Content[0], theirs.Content[0], path, conflicts)
		if err != nil {
			return nil, err
		}
		result.Content = []*yaml.Node{merged}
	case len(ours.Content) > 0 && len(theirs.Content) > 0:
		// Base is empty but both sides have content: concurrent additions to a
		// new file.  Use an empty placeholder as the common ancestor so the
		// normal merge logic can handle or record the conflict rather than
		// silently discarding theirs.
		emptyBase := createEmptyNodeOfKind(ours.Content[0])
		merged, err := m.mergeNodes(emptyBase, ours.Content[0], theirs.Content[0], path, conflicts)
		if err != nil {
			return nil, err
		}
		result.Content = []*yaml.Node{merged}
	case len(ours.Content) > 0:
		result.Content = ours.Content
	case len(theirs.Content) > 0:
		result.Content = theirs.Content
	}

	return result, nil
}

// mergeMappings merges mapping (object) nodes with key-level intelligence.
//
//nolint:gocognit,revive,cyclop,funlen // inherently complex 3-way merge algorithm
func (m *YAMLMerger) mergeMappings(base, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	// Detect mapping vs non-mapping kind mismatches before building key maps.
	// If any of base/ours/theirs is not a MappingNode while another is,
	// the user or template changed the node type - record a conflict and return the user's node.
	baseIsMapping := base.Kind == yaml.MappingNode
	oursIsMapping := ours.Kind == yaml.MappingNode
	theirsIsMapping := theirs.Kind == yaml.MappingNode

	// If there's a kind mismatch, resolve per the configured conflict strategy.
	if !baseIsMapping || !oursIsMapping || !theirsIsMapping {
		return m.pickConflictValue(ours, theirs, path, conflicts), nil
	}

	result := &yaml.Node{
		Kind:        yaml.MappingNode,
		Tag:         ours.Tag,
		Style:       ours.Style,
		HeadComment: ours.HeadComment,
		LineComment: ours.LineComment,
		FootComment: ours.FootComment,
		Content:     make([]*yaml.Node, 0),
	}

	// Build map for base lookup
	baseMap, err := buildKeyMap(base, path)
	if err != nil {
		return nil, err
	}
	// Build map for theirs lookup (checking if keys exist)
	theirsMap, err := buildKeyMap(theirs, path)
	if err != nil {
		return nil, err
	}

	// Track which keys we've processed
	processed := make(map[string]bool)

	// Process keys from ours first (preserve user's key order)
	// Iterate over ours.Content directly to maintain order (maps are unordered in Go)
	for i := 0; i < len(ours.Content); i += 2 {
		if i+1 >= len(ours.Content) {
			break
		}

		keyNode := ours.Content[i]
		oursValue := ours.Content[i+1]
		key := keyNode.Value

		processed[key] = true

		baseValue, inBase := baseMap[key]
		theirsValue, inTheirs := theirsMap[key]

		keyPath := path
		if keyPath != "" {
			keyPath += "."
		}
		keyPath += key

		switch {
		case !inBase && !inTheirs:
			// User added, not in template - keep it (with comments).
			result.Content = append(result.Content, keyNode, oursValue)
		case !inBase && inTheirs:
			// Both added the same key - merge values.
			if oursValue.Kind != theirsValue.Kind {
				picked := m.pickConflictValue(oursValue, theirsValue, keyPath, conflicts)
				result.Content = append(result.Content, keyNode, picked)
				continue
			}

			basePlaceholder := createEmptyNodeOfKind(oursValue)
			merged, err := m.mergeNodes(basePlaceholder, oursValue, theirsValue, keyPath, conflicts)
			if err != nil {
				return nil, err
			}
			result.Content = append(result.Content, keyNode, merged)
		case inBase && !inTheirs:
			// User modified, template deleted - keep user's version (preserve user changes and comments).
			result.Content = append(result.Content, keyNode, oursValue)
		default:
			// All three have the key - merge values (preserve user's key comments).
			merged, err := m.mergeNodes(baseValue, oursValue, theirsValue, keyPath, conflicts)
			if err != nil {
				return nil, err
			}
			result.Content = append(result.Content, keyNode, merged)
		}
	}

	// Add keys from theirs that aren't in ours (template additions)
	// Iterate over theirs.Content directly to preserve key comments
	for i := 0; i < len(theirs.Content); i += 2 {
		if i+1 >= len(theirs.Content) {
			break
		}

		theirKeyNode := theirs.Content[i]
		theirValue := theirs.Content[i+1]
		key := theirKeyNode.Value

		if processed[key] {
			continue
		}
		processed[key] = true

		_, inBase := baseMap[key]

		if !inBase {
			// Template added new key - include it (with template's comments)
			result.Content = append(result.Content, theirKeyNode, theirValue)
		}
		// If it was in base but not ours, user deleted it - respect deletion
	}

	return result, nil
}

// mergeSequences merges sequence (array) nodes.
func (m *YAMLMerger) mergeSequences(_, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	// For sequences, a real divergence is resolved per the configured
	// conflict strategy (manual/ours/theirs); identical sequences need no choice.
	picked := ours
	if !nodesEqual(ours, theirs) {
		picked = m.pickConflictValue(ours, theirs, path, conflicts)
	}

	// Preserve the picked side's comments and style.
	result := &yaml.Node{
		Kind:        yaml.SequenceNode,
		Tag:         picked.Tag,
		Style:       picked.Style,
		HeadComment: picked.HeadComment,
		LineComment: picked.LineComment,
		FootComment: picked.FootComment,
		Content:     picked.Content,
	}

	return result, nil
}

// mergeScalars merges scalar (primitive value) nodes.
func (m *YAMLMerger) mergeScalars(base, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	// A real divergence (both changed to different values) is resolved per
	// the configured conflict strategy (manual/ours/theirs).
	picked := ours
	if ours.Value != base.Value && theirs.Value != base.Value && ours.Value != theirs.Value {
		picked = m.pickConflictValue(ours, theirs, path, conflicts)
	}

	// Preserve the picked side's comments, tag, and style (folding, literal, etc.)
	result := &yaml.Node{
		Kind:        yaml.ScalarNode,
		Tag:         picked.Tag,
		Style:       picked.Style,
		Value:       picked.Value,
		HeadComment: picked.HeadComment,
		LineComment: picked.LineComment,
		FootComment: picked.FootComment,
	}

	return result, nil
}

// buildKeyMap builds a map of key -> value node for a mapping.
// Non-scalar keys (complex keys) cannot be represented as string keys:
// silently skipping them would drop or mis-merge their entries, so they are
// reported as an explicit error instead. Complex YAML keys are out of scope
// for Atmos scaffold merges.
func buildKeyMap(node *yaml.Node, path string) (map[string]*yaml.Node, error) {
	result := make(map[string]*yaml.Node)

	if node.Kind != yaml.MappingNode {
		return result, nil
	}

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, errUtils.Build(errUtils.ErrMergeConflict).
				WithExplanationf("Mapping at `%s` uses a complex (non-scalar) YAML key, which is not supported by the 3-way merge", displayPath(path)).
				WithHint("Replace complex mapping keys with plain string keys").
				WithContext("path", displayPath(path)).
				WithContext("key_kind", fmt.Sprintf("%d", keyNode.Kind)).
				WithExitCode(2).
				Err()
		}
		key := keyNode.Value
		value := node.Content[i+1]
		result[key] = value
	}

	return result, nil
}

// displayPath renders a node path for error messages, naming the document
// root explicitly when the path is empty.
func displayPath(path string) string {
	if path == "" {
		return "(document root)"
	}
	return path
}

// createEmptyNodeOfKind creates an empty placeholder node matching the given node's kind.
func createEmptyNodeOfKind(node *yaml.Node) *yaml.Node {
	placeholder := &yaml.Node{
		Kind:  node.Kind,
		Tag:   node.Tag,
		Style: node.Style,
	}

	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode, yaml.DocumentNode:
		placeholder.Content = make([]*yaml.Node, 0)
	case yaml.ScalarNode:
		placeholder.Value = ""
	}

	return placeholder
}

// nodesEqual checks if two YAML nodes are structurally equal.
//
//nolint:gocognit,revive,cyclop // recursive node comparison requires switch over node types
func nodesEqual(a, b *yaml.Node) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case yaml.ScalarNode:
		// Tag affects semantics (e.g., !!bool vs !!str), so include it in comparison.
		return a.Tag == b.Tag && a.Value == b.Value
	case yaml.MappingNode, yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodesEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	case yaml.DocumentNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodesEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

// calculateYAMLChangePercentage calculates change percentage for YAML conflicts.
func (m *YAMLMerger) calculateYAMLChangePercentage(base, _, _ string, conflictCount int) int {
	// Count total keys across every document in base as a rough measure of
	// content size — a multi-document base must sum keys from all documents,
	// not just the first, or the denominator undercounts and change% is
	// overstated for multi-document streams.
	baseDocs, err := decodeAllDocuments(base, "base")
	if err != nil {
		return maxChangePercentage // If we can't parse, assume high change
	}

	totalKeys := 0
	for _, doc := range baseDocs {
		totalKeys += countKeys(doc)
	}
	if totalKeys == 0 {
		totalKeys = 1 // Avoid division by zero
	}

	return int(float64(conflictCount) / float64(totalKeys) * 100.0)
}

// countKeys counts the total number of keys in a YAML document.
func countKeys(node *yaml.Node) int {
	switch node.Kind {
	case yaml.DocumentNode:
		count := 0
		for _, child := range node.Content {
			count += countKeys(child)
		}
		return count
	case yaml.MappingNode:
		// Each key-value pair counts as 1 key
		count := len(node.Content) / 2
		for i := 1; i < len(node.Content); i += 2 {
			count += countKeys(node.Content[i])
		}
		return count
	case yaml.SequenceNode:
		count := 0
		for _, child := range node.Content {
			count += countKeys(child)
		}
		return count
	default:
		return 0
	}
}
