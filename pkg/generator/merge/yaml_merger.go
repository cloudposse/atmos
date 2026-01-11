package merge

import (
	"bytes"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Constants for YAML merging.
const (
	maxChangePercentage = 100 // Maximum change percentage when parsing fails
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
	thresholdPercent int // Percentage threshold (0-100) for change detection
}

// NewYAMLMerger creates a new YAML merger with the specified percentage threshold.
func NewYAMLMerger(thresholdPercent int) *YAMLMerger {
	defer perf.Track(nil, "merge.NewYAMLMerger")()

	return &YAMLMerger{
		thresholdPercent: thresholdPercent,
	}
}

// Merge performs a 3-way merge of YAML content with structure awareness.
// Parameters:
//   - base: The original YAML content (common ancestor)
//   - ours: The user's YAML version (with their changes)
//   - theirs: The template's YAML version (with template updates)
//
// Returns the merged YAML content or an error if conflicts exceed threshold.
func (m *YAMLMerger) Merge(base, ours, theirs string) (*MergeResult, error) {
	defer perf.Track(nil, "merge.YAMLMerger.Merge")()

	// Parse all three YAML documents
	var baseNode, oursNode, theirsNode yaml.Node

	if err := yaml.Unmarshal([]byte(base), &baseNode); err != nil {
		return nil, fmt.Errorf("failed to parse base YAML: %w", err)
	}

	if err := yaml.Unmarshal([]byte(ours), &oursNode); err != nil {
		return nil, fmt.Errorf("failed to parse ours YAML: %w", err)
	}

	if err := yaml.Unmarshal([]byte(theirs), &theirsNode); err != nil {
		return nil, fmt.Errorf("failed to parse theirs YAML: %w", err)
	}

	// Perform structure-aware merge
	conflicts := &conflictTracker{conflicts: make([]string, 0)}
	mergedNode, err := m.mergeNodes(&baseNode, &oursNode, &theirsNode, "", conflicts)
	if err != nil {
		return nil, fmt.Errorf("failed to merge YAML structures: %w", err)
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

	// Marshal back to YAML, preserving formatting
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(mergedNode); err != nil {
		return nil, fmt.Errorf("failed to encode merged YAML: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to close YAML encoder: %w", err)
	}

	return &MergeResult{
		Content:       buf.String(),
		HasConflicts:  len(conflicts.conflicts) > 0,
		ConflictCount: len(conflicts.conflicts),
	}, nil
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

	// Handle based on node type
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
		// For other node types, prefer ours if different
		return ours, nil
	}
}

// mergeDocuments merges document nodes.
func (m *YAMLMerger) mergeDocuments(base, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	result := &yaml.Node{
		Kind: yaml.DocumentNode,
	}

	// Merge the content nodes
	if len(base.Content) > 0 && len(ours.Content) > 0 && len(theirs.Content) > 0 {
		merged, err := m.mergeNodes(base.Content[0], ours.Content[0], theirs.Content[0], path, conflicts)
		if err != nil {
			return nil, err
		}
		result.Content = []*yaml.Node{merged}
	} else if len(ours.Content) > 0 {
		result.Content = ours.Content
	} else if len(theirs.Content) > 0 {
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

	// If there's a kind mismatch, record conflict and preserve user's choice.
	if !baseIsMapping || !oursIsMapping || !theirsIsMapping {
		conflicts.addConflict(path)
		return ours, nil
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
	baseMap := buildKeyMap(base)
	// Build map for theirs lookup (checking if keys exist)
	theirsMap := buildKeyMap(theirs)

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

		if !inBase && !inTheirs {
			// User added, not in template - keep it (with comments)
			result.Content = append(result.Content, keyNode, oursValue)
		} else if !inBase && inTheirs {
			// Both added the same key - merge values
			if oursValue.Kind != theirsValue.Kind {
				conflicts.addConflict(keyPath)
				result.Content = append(result.Content, keyNode, oursValue)
				continue
			}

			basePlaceholder := createEmptyNodeOfKind(oursValue)
			merged, err := m.mergeNodes(basePlaceholder, oursValue, theirsValue, keyPath, conflicts)
			if err != nil {
				return nil, err
			}
			result.Content = append(result.Content, keyNode, merged)
		} else if inBase && !inTheirs {
			// User modified, template deleted - keep user's version (preserve user changes and comments)
			result.Content = append(result.Content, keyNode, oursValue)
		} else {
			// All three have the key - merge values (preserve user's key comments)
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
	// For sequences, if they differ, it's a conflict
	// We prefer ours (user's version) unless they're identical
	if !nodesEqual(ours, theirs) {
		conflicts.addConflict(path)
	}

	// Preserve user's version with comments and style
	result := &yaml.Node{
		Kind:        yaml.SequenceNode,
		Tag:         ours.Tag,
		Style:       ours.Style,
		HeadComment: ours.HeadComment,
		LineComment: ours.LineComment,
		FootComment: ours.FootComment,
		Content:     ours.Content,
	}

	return result, nil
}

// mergeScalars merges scalar (primitive value) nodes.
func (m *YAMLMerger) mergeScalars(base, ours, theirs *yaml.Node, path string, conflicts *conflictTracker) (*yaml.Node, error) {
	// If both changed to different values, it's a conflict
	if ours.Value != base.Value && theirs.Value != base.Value && ours.Value != theirs.Value {
		conflicts.addConflict(path)
	}

	// Preserve user's value with their comments, tag, and style (folding, literal, etc.)
	result := &yaml.Node{
		Kind:        yaml.ScalarNode,
		Tag:         ours.Tag,
		Style:       ours.Style,
		Value:       ours.Value,
		HeadComment: ours.HeadComment,
		LineComment: ours.LineComment,
		FootComment: ours.FootComment,
	}

	return result, nil
}

// buildKeyMap builds a map of key -> value node for a mapping.
func buildKeyMap(node *yaml.Node) map[string]*yaml.Node {
	result := make(map[string]*yaml.Node)

	if node.Kind != yaml.MappingNode {
		return result
	}

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			key := node.Content[i].Value
			value := node.Content[i+1]
			result[key] = value
		}
	}

	return result
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
		return a.Value == b.Value
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
	// Count total keys in base as rough measure of content size
	var baseNode yaml.Node
	if err := yaml.Unmarshal([]byte(base), &baseNode); err != nil {
		return maxChangePercentage // If we can't parse, assume high change
	}

	totalKeys := countKeys(&baseNode)
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
