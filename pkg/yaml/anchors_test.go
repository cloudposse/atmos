package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers anchorViolation branches not exercised by the higher-level
// Set/Eval tests in edit_test.go and regression_test.go, plus recursive
// duplicate-anchor detection nested inside a sequence inside a mapping.

// --- anchorViolation branches -------------------------------------------------

func TestAnchorViolation_RemovedOrExpanded(t *testing.T) {
	before := map[string]anchorInfo{"x": {content: "1"}}
	after := map[string]anchorInfo{}

	msg := anchorViolation("x", before, after)
	assert.Contains(t, msg, "anchor &x was removed or expanded by the edit")
}

func TestAnchorViolation_Introduced(t *testing.T) {
	before := map[string]anchorInfo{}
	after := map[string]anchorInfo{"x": {content: "1"}}

	msg := anchorViolation("x", before, after)
	assert.Contains(t, msg, "anchor &x was introduced by the edit")
}

func TestAnchorViolation_AliasCountChanged(t *testing.T) {
	before := map[string]anchorInfo{"x": {content: "1", aliasCount: 2}}
	after := map[string]anchorInfo{"x": {content: "1", aliasCount: 3}}

	msg := anchorViolation("x", before, after)
	assert.Contains(t, msg, "anchor &x alias references changed from 2 to 3")
}

func TestAnchorViolation_NoChange(t *testing.T) {
	before := map[string]anchorInfo{"x": {content: "1", aliasCount: 1}}
	after := map[string]anchorInfo{"x": {content: "1", aliasCount: 1}}

	assert.Empty(t, anchorViolation("x", before, after))
}

// --- Recursive duplicate-anchor detection -------------------------------------

// TestCollectAnchors_DuplicateAnchor_NestedInSequenceInsideMapping verifies that
// a duplicate anchor definition nested a few levels deep (inside a sequence
// element, inside a mapping) is still detected: the recursive error return from
// walkAnchorNodes must propagate up through every level of the child loop.
func TestCollectAnchors_DuplicateAnchor_NestedInSequenceInsideMapping(t *testing.T) {
	nested := `top:
  list:
    - name: first
      value: &dup 1
    - name: second
      value: &dup 2
`
	_, err := collectAnchors([]byte(nested))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrYAMLDuplicateAnchor)
	assert.Contains(t, err.Error(), "&dup")
}

// TestCollectAnchors_NoDuplicates_NestedAnchorsAreRecorded is a sanity check that
// the same nested shape without a duplicate anchor name parses cleanly and both
// anchors are recorded, confirming the recursive walk itself (not just the
// duplicate-error path) reaches nested sequence/mapping content.
func TestCollectAnchors_NoDuplicates_NestedAnchorsAreRecorded(t *testing.T) {
	nested := `top:
  list:
    - name: first
      value: &one 1
    - name: second
      value: &two 2
`
	anchors, err := collectAnchors([]byte(nested))
	require.NoError(t, err)
	assert.Contains(t, anchors, "one")
	assert.Contains(t, anchors, "two")
}
