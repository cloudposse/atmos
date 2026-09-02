package tree

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnector(t *testing.T) {
	tests := []struct {
		name string
		path Path
		want string
	}{
		{"root child, last", Path{true}, "└── "},
		{"root child, more", Path{false}, "├── "},
		{"under a last parent", Path{true, false}, "    ├── "},
		{"under a parent with siblings", Path{false, true}, "│   └── "},
		{"empty path", Path{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Connector(tt.path))
		})
	}
}

func TestContentGutter(t *testing.T) {
	tests := []struct {
		name        string
		path        Path
		hasChildren bool
		want        string
	}{
		{"leaf, last", Path{true}, false, "        "},
		{"leaf, more siblings keeps own rail", Path{false}, false, "│       "},
		{"parent, last: rail only to children", Path{true}, true, "    │   "},
		{"parent with siblings: own rail and child rail", Path{false}, true, "│   │   "},
		{"nested parent", Path{true, false}, true, "    │   │   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ContentGutter(tt.path, tt.hasChildren))
		})
	}
}

func TestSpacerGutter(t *testing.T) {
	assert.Equal(t, "│", SpacerGutter(Path{false}))
	assert.Equal(t, "", SpacerGutter(Path{true}))
	assert.Equal(t, "│       │", SpacerGutter(Path{false, true, false}))
}

func TestSpacerFromConnectorRow(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want string
	}{
		{"root-level spacer with siblings after it", "├── <<<SPACER>>>", "│"},
		{"root-level spacer as last child", "└── <<<SPACER>>>", ""},
		{"nested spacer keeps the ancestor rail and adds its own", "│   ├── <<<SPACER>>>", "│   │"},
		{"nested spacer under a last parent", "    ├── <<<SPACER>>>", "    │"},
		{"nested last spacer keeps only the ancestor rail", "│   └── <<<SPACER>>>", "│"},
		{"leading fixed column is preserved", "  ●  │   ├── x", "  ●  │   │"},
		{"no connector", "just text", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SpacerFromConnectorRow(tt.row))
		})
	}
}

func TestViolations_Connected(t *testing.T) {
	rows := []string{
		"  dev/vpc",
		"  └── vpc",
		"      │   attr",
		"      ├── subnet",
		"      │   │   attr",
		"      │   └── rta",
		"      └── other",
	}
	assert.Empty(t, Violations(rows))
}

// TestViolations_LipglossThreeWideArms verifies trees rendered by lipgloss/tree, whose
// default enumerator arm is three columns wide, are recognized as connected.
func TestViolations_LipglossThreeWideArms(t *testing.T) {
	rows := []string{
		"│",
		"├──stack-a",
		"│  ├──eks",
		"│  │  └──catalog/eks",
		"│  │",
		"│  └──vpc",
		"│",
		"└──stack-b",
		"   └──vpc",
	}
	assert.Empty(t, Violations(rows))
}

func TestViolations_FloatingChild(t *testing.T) {
	rows := []string{
		"  └── vpc",
		"          attr", // no rail down to the child
		"      └── subnet",
	}
	v := Violations(rows)
	require.Len(t, v, 1)
	assert.Contains(t, v[0], "row 2 col 6")
}

func TestViolations_BrokenRail(t *testing.T) {
	rows := []string{
		"  ├── a",
		"  │   attr",
		"      attr", // rail dropped for one row
		"  │   attr",
		"  └── b",
	}
	assert.Len(t, Violations(rows), 1)
}

// node is a minimal tree used to render arbitrary shapes with the package's gutters.
type node struct {
	content  int
	children []*node
}

// render is a reference renderer that uses nothing but this package's gutters, so the
// property test exercises the geometry itself rather than any particular caller.
func render(b *strings.Builder, nodes []*node, path Path, spacers bool) {
	for i, n := range nodes {
		p := append(append(Path{}, path...), i == len(nodes)-1)
		b.WriteString("  " + Connector(p) + "node\n")
		for j := 0; j < n.content; j++ {
			b.WriteString("  " + ContentGutter(p, len(n.children) > 0) + "attr\n")
		}
		render(b, n.children, p, spacers)
		if spacers && i < len(nodes)-1 {
			b.WriteString("  " + SpacerGutter(p) + "\n")
		}
	}
}

func gen(r *rand.Rand, depth int) []*node {
	if depth == 0 {
		return nil
	}
	nodes := make([]*node, r.Intn(4))
	for i := range nodes {
		nodes[i] = &node{content: r.Intn(4), children: gen(r, depth-1)}
	}
	return nodes
}

// TestViolations_Property renders many random trees (fan-out 0-3, depth up to 4, 0-3
// content rows per node, with and without spacers) and asserts every one is connected.
func TestViolations_Property(t *testing.T) {
	r := rand.New(rand.NewSource(1)) // Fixed seed: deterministic test data.
	for i := 0; i < 500; i++ {
		var b strings.Builder
		nodes := gen(r, 1+r.Intn(4))
		b.WriteString("  root\n")
		render(&b, nodes, nil, i%2 == 0)
		rows := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
		if v := Violations(rows); len(v) != 0 {
			t.Fatalf("tree %d is not connected:\n%s\n%s", i, strings.Join(v, "\n"), b.String())
		}
	}
}
