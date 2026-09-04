package releasenotes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureBody = `<details>
  <summary>feat: add widget @alice (#101)</summary>
## what

Adds a widget.

## why

Because widgets.
</details>

## 🚀 Enhancements

<details>
  <summary>fix: correct gizmo @bob (#102)</summary>
Fixes the gizmo off-by-one.
</details>

<details>
  <summary>fix: another gizmo fix @carol (#103)</summary>
Also fixes the gizmo.
</details>
`

func TestParseDraftedBody(t *testing.T) {
	entries, err := ParseDraftedBody(fixtureBody)
	require.NoError(t, err)
	require.Len(t, entries, 3)

	first := entries[0]
	assert.Equal(t, "", first.Category)
	assert.Equal(t, "feat: add widget @alice (#101)", first.Summary)
	assert.Equal(t, 101, first.Number)
	assert.Contains(t, first.Body, "Adds a widget.")

	last := entries[2]
	assert.Equal(t, "## 🚀 Enhancements", last.Category)
	assert.Equal(t, 103, last.Number)
	assert.Contains(t, last.Body, "Also fixes the gizmo.")
}

func TestParseDraftedBody_CategoryAppliesToFollowingEntries(t *testing.T) {
	entries, err := ParseDraftedBody(fixtureBody)
	require.NoError(t, err)
	require.Len(t, entries, 3)

	// The category heading sits between entry 0 and entry 1, so it must not
	// leak backwards onto the entry parsed before it.
	assert.Equal(t, "", entries[0].Category)
	assert.Equal(t, "## 🚀 Enhancements", entries[1].Category)
	assert.Equal(t, "## 🚀 Enhancements", entries[2].Category)
}

func TestParseDraftedBody_SkipsBlockWithNoSummaryLine(t *testing.T) {
	body := `<details>
  no summary tag in this block, just stray text
</details>

<details>
  <summary>feat: valid entry @alice (#5)</summary>
body
</details>
`
	entries, err := ParseDraftedBody(body)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 5, entries[0].Number)
}

func TestParseDraftedBody_IgnoresHeadingsInsidePriorEntryBody(t *testing.T) {
	// The first entry's own PR body contains a "## why" heading. It must not
	// leak forward and become the category of the second, otherwise
	// uncategorized entry.
	body := `<details>
  <summary>feat: first @alice (#1)</summary>
## why

Because reasons.
</details>

<details>
  <summary>fix: second @bob (#2)</summary>
body
</details>
`
	entries, err := ParseDraftedBody(body)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "", entries[0].Category)
	assert.Equal(t, "", entries[1].Category)
}

func TestParseDraftedBody_Errors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "no details blocks", body: "## Enhancements\n\nJust text, no PR entries.\n"},
		{name: "unbalanced details", body: "<details>\n  <summary>x (#1)</summary>\nbody\n"},
		{
			// Equal marker counts (two opens, two closes) but out of order: the
			// first </details> closes before the first <details> even opens.
			// This must be rejected rather than panic on a low > high slice.
			name: "balanced but out-of-order details",
			body: "</details>\n<details>\n  <summary>x (#1)</summary>\n</details>\n<details>\n  <summary>y (#2)</summary>\nbody\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDraftedBody(tt.body)
			require.Error(t, err)
			assert.ErrorIs(t, err, errNoEntries)
		})
	}
}

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		want    int
	}{
		{name: "standard release-drafter format", summary: "feat: thing @author (#42)", want: 42},
		{name: "no trailing number", summary: "feat: thing with no number", want: 0},
		{name: "number not at end", summary: "#42 feat: thing @author", want: 0},
		{name: "number too large to fit in int", summary: "feat: thing (#99999999999999999999)", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractPRNumber(tt.summary))
		})
	}
}
