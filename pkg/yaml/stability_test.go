package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "gopkg.in/yaml.v3"
)

// This file holds the YAML STABILITY suite: edits must preserve comments and
// anchors/aliases across a wide variety of YAML constructs. "Stability" here
// means an edit changes only its target and otherwise round-trips the document
// faithfully (comments survive, anchor topology is unchanged).

// countAliases returns how many alias nodes reference each anchor name.
func countAliases(t *testing.T, content []byte) map[string]int {
	t.Helper()
	var root goyaml.Node
	require.NoError(t, goyaml.Unmarshal(content, &root))
	counts := map[string]int{}
	var walk func(n *goyaml.Node)
	walk = func(n *goyaml.Node) {
		if n == nil {
			return
		}
		if n.Kind == goyaml.AliasNode {
			counts[n.Value]++
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)
	return counts
}

// allComments extracts every comment string present in a document so tests can
// assert none were dropped.
func allComments(t *testing.T, content []byte) []string {
	t.Helper()
	var root goyaml.Node
	require.NoError(t, goyaml.Unmarshal(content, &root))
	var comments []string
	var walk func(n *goyaml.Node)
	walk = func(n *goyaml.Node) {
		if n == nil {
			return
		}
		for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
			if c != "" {
				comments = append(comments, c)
			}
		}
		for _, ch := range n.Content {
			walk(ch)
		}
	}
	walk(&root)
	return comments
}

// kitchenSink exercises a broad cross-section of YAML features.
const kitchenSink = `# Document header comment.
# Spanning two lines.
metadata:
  name: example  # name of the thing
  # standalone comment before labels
  labels:
    app: web
    tier: frontend

# An anchor that is reused below.
defaults: &defaults
  retries: 3
  timeout: 30  # seconds

services:
  web:
    <<: *defaults  # merge defaults
    port: 8080
  api:
    <<: *defaults
    port: 9090
    # foot comment on api

config:
  # block scalar (literal)
  script: |
    #!/bin/sh
    echo "hello"
  # folded scalar
  description: >
    a long
    description
  flow_map: {a: 1, b: 2}  # inline flow map
  flow_seq: [one, two, three]
  quoted_single: 'single ''quoted'' value'
  quoted_double: "double \"quoted\" value"
  unicode: "café — naïve"
  empty:
  nullval: null

list_of_maps:
  - name: first
    value: 1
  - name: second  # second item
    value: 2
`

func TestStability_KitchenSinkSetPreservesEverything(t *testing.T) {
	beforeComments := allComments(t, []byte(kitchenSink))
	beforeAliases := countAliases(t, []byte(kitchenSink))
	require.NotEmpty(t, beforeComments, "fixture must contain comments")
	require.Equal(t, 2, beforeAliases["defaults"], "fixture must have 2 aliases of &defaults")

	// Edit a value far from any anchor.
	out, err := Set([]byte(kitchenSink), "metadata.labels.app", "api-gateway")
	require.NoError(t, err)

	// Target changed.
	assert.Contains(t, string(out), "api-gateway")

	// Every comment survived.
	afterComments := allComments(t, out)
	for _, c := range beforeComments {
		assert.Containsf(t, afterComments, c, "comment dropped: %q", c)
	}

	// Anchor topology unchanged.
	assert.Equal(t, beforeAliases, countAliases(t, out), "alias topology must be unchanged")
	assert.Contains(t, string(out), "&defaults", "anchor definition preserved")

	// Block and flow constructs survive.
	assert.Contains(t, string(out), "#!/bin/sh", "literal block scalar preserved")
	assert.Contains(t, string(out), "flow_map:", "flow map key preserved")
}

func TestStability_MergeKeyEditIsRejected(t *testing.T) {
	// services.web inherits via <<: *defaults. Trying to set a key that lives in
	// the merged anchor must be guarded (it would mutate shared &defaults).
	_, err := Set([]byte(kitchenSink), "defaults.retries", "5")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrYAMLAnchorAltered)
}

func TestStability_DeletePreservesUnrelatedCommentsAndAnchors(t *testing.T) {
	beforeAliases := countAliases(t, []byte(kitchenSink))

	out, err := Delete([]byte(kitchenSink), "metadata.labels.tier")
	require.NoError(t, err)

	assert.NotContains(t, string(out), "tier: frontend")
	assert.Contains(t, string(out), "# Document header comment.")
	assert.Contains(t, string(out), "&defaults")
	assert.Equal(t, beforeAliases, countAliases(t, out), "delete must not disturb anchors")
}

func TestStability_FormatIsIdempotentAndPreserving(t *testing.T) {
	once, err := Format([]byte(kitchenSink))
	require.NoError(t, err)
	twice, err := Format(once)
	require.NoError(t, err)
	assert.Equal(t, string(once), string(twice), "Format must be idempotent")

	// Comments and anchors survive formatting.
	for _, c := range allComments(t, []byte(kitchenSink)) {
		assert.Containsf(t, allComments(t, once), c, "format dropped comment: %q", c)
	}
	assert.Equal(t, countAliases(t, []byte(kitchenSink)), countAliases(t, once))
}

// Table of focused stability fixtures, each asserting a single property holds
// after a benign edit.
func TestStability_Fixtures(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		path    string
		value   string
		wantSub string // must appear after edit
		comment string // a comment that must survive
	}{
		{
			name:    "comment on sequence item",
			yaml:    "items:\n  - a  # first\n  - b  # second\nname: x\n",
			path:    "name",
			value:   "y",
			wantSub: "name: y",
			comment: "# first",
		},
		{
			name:    "deeply nested keys",
			yaml:    "a:\n  b:\n    c:\n      d: 1  # deep\nother: 0\n",
			path:    "other",
			value:   "5",
			wantSub: "other:",
			comment: "# deep",
		},
		{
			name:    "anchor on a mapping reused twice",
			yaml:    "base: &b\n  x: 1\nuse1: *b\nuse2: *b\ntoggle: false\n",
			path:    "toggle",
			value:   "true",
			wantSub: "toggle:",
			comment: "",
		},
		{
			name:    "preserve double-quoted style neighbor",
			yaml:    "a: \"keep me\"  # q\nb: plain\n",
			path:    "b",
			value:   "changed",
			wantSub: "changed",
			comment: "# q",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Set([]byte(tt.yaml), tt.path, tt.value)
			require.NoError(t, err)
			s := string(out)
			assert.Contains(t, s, tt.wantSub)
			if tt.comment != "" {
				assert.Contains(t, s, tt.comment, "comment must survive")
			}
			// Anchor topology preserved.
			assert.Equal(t, countAliases(t, []byte(tt.yaml)), countAliases(t, out))
			// Double-quoted neighbor keeps its quoting.
			if strings.Contains(tt.yaml, `"keep me"`) {
				assert.Contains(t, s, `"keep me"`)
			}
		})
	}
}
