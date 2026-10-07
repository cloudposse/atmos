package vulncheck

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dedupe runs DedupeStacks over input and returns the report it wrote.
func dedupe(t *testing.T, input string) string {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, DedupeStacks(strings.NewReader(input), &out))
	return out.String()
}

// stackCounts returns the number of stacks in each result of the first run.
func stackCounts(t *testing.T, report string) []int {
	t.Helper()

	var doc struct {
		Runs []struct {
			Results []struct {
				Stacks []json.RawMessage `json:"stacks"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &doc))
	require.NotEmpty(t, doc.Runs)

	counts := make([]int, 0, len(doc.Runs[0].Results))
	for _, result := range doc.Runs[0].Results {
		counts = append(counts, len(result.Stacks))
	}
	return counts
}

func TestDedupeStacks_RemovesOnlyExactDuplicates(t *testing.T) {
	// Objects that differ only in key order are the same stack. A different
	// message, or the same frames in another order, is a different stack.
	input := `{
		"version": "2.1.0",
		"runs": [{
			"tool": {"driver": {"name": "govulncheck"}},
			"results": [{
				"ruleId": "GO-TEST",
				"level": "error",
				"message": {"text": "Keep this finding"},
				"stacks": [
					{"message": {"text": "trace"}, "frames": [{"module": "b"}, {"module": "a"}]},
					{"frames": [{"module": "b"}, {"module": "a"}], "message": {"text": "trace"}},
					{"message": {"text": "other"}, "frames": [{"module": "b"}, {"module": "a"}]},
					{"message": {"text": "trace"}, "frames": [{"module": "a"}, {"module": "b"}]},
					{"message": {"text": "trace"}, "frames": [{"module": "b"}, {"module": "a"}]}
				],
				"codeFlows": [{"message": {"text": "unchanged"}}]
			}]
		}]
	}`
	want := `{
		"version": "2.1.0",
		"runs": [{
			"tool": {"driver": {"name": "govulncheck"}},
			"results": [{
				"ruleId": "GO-TEST",
				"level": "error",
				"message": {"text": "Keep this finding"},
				"stacks": [
					{"message": {"text": "trace"}, "frames": [{"module": "b"}, {"module": "a"}]},
					{"message": {"text": "other"}, "frames": [{"module": "b"}, {"module": "a"}]},
					{"message": {"text": "trace"}, "frames": [{"module": "a"}, {"module": "b"}]}
				],
				"codeFlows": [{"message": {"text": "unchanged"}}]
			}]
		}]
	}`

	assert.JSONEq(t, want, dedupe(t, input))
}

func TestDedupeStacks_KeepsFirstOccurrencePosition(t *testing.T) {
	got := dedupe(t, `{"runs":[{"results":[{"stacks":[{"id":3},{"id":1},{"id":3},{"id":2},{"id":1}]}]}]}`)

	assert.JSONEq(t, `{"runs":[{"results":[{"stacks":[{"id":3},{"id":1},{"id":2}]}]}]}`, got)
}

func TestDedupeStacks_DeduplicatesEveryResultAndRun(t *testing.T) {
	got := dedupe(t, `{"runs":[
		{"results":[{"stacks":[{"a":1},{"a":1}]},{"stacks":[{"b":1},{"b":1},{"b":1}]}]},
		{"results":[{"stacks":[{"c":1},{"c":1}]}]}
	]}`)

	assert.JSONEq(t, `{"runs":[
		{"results":[{"stacks":[{"a":1}]},{"stacks":[{"b":1}]}]},
		{"results":[{"stacks":[{"c":1}]}]}
	]}`, got)
}

func TestDedupeStacks_LeavesUnrelatedShapesAlone(t *testing.T) {
	// None of these carry a duplicate-able stacks array, so the report must come
	// back unchanged and without an error.
	tests := []struct {
		name  string
		input string
	}{
		{"result without stacks", `{"runs":[{"results":[{"ruleId":"A","message":{"text":"x"}}]}]}`},
		{"empty stacks", `{"runs":[{"results":[{"stacks":[]}]}]}`},
		{"null stacks", `{"runs":[{"results":[{"stacks":null}]}]}`},
		{"string stacks", `{"runs":[{"results":[{"stacks":"x"}]}]}`},
		{"scalar elements are not stacks", `{"runs":[{"results":[7, null, "x"]}]}`},
		{"run without results", `{"runs":[{"tool":{"driver":{"name":"x"}}}]}`},
		{"null results", `{"runs":[{"results":null}]}`},
		{"empty runs", `{"version":"2.1.0","runs":[]}`},
		{"null runs", `{"runs":null}`},
		{"no runs", `{"version":"2.1.0"}`},
		{"scalar run", `{"runs":[3]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.JSONEq(t, tt.input, dedupe(t, tt.input))
		})
	}
}

func TestDedupeStacks_IsIdempotent(t *testing.T) {
	once := dedupe(t, `{"runs":[{"results":[{"stacks":[{"a":1},{"a":1},{"b":2}]},{"ruleId":"NO-STACKS"}]}]}`)
	twice := dedupe(t, once)

	assert.Equal(t, once, twice)
	assert.Equal(t, []int{2, 0}, stackCounts(t, twice))
}

func TestDedupeStacks_PreservesNumbersAndText(t *testing.T) {
	// Integers beyond float64 precision, trailing zeros and HTML-sensitive text
	// must survive the round trip untouched.
	input := `{"runs":[{"results":[{"stacks":[{"line":12345678901234567890,"ratio":1.50,"text":"a<b && c>d"}]}]}]}`
	got := dedupe(t, input)

	assert.Contains(t, got, "12345678901234567890")
	assert.Contains(t, got, "1.50")
	assert.Contains(t, got, "a<b && c>d")
	assert.NotContains(t, got, "\\u003c", "HTML characters must not be escaped")
}

func TestDedupeStacks_DistinguishesDifferentNumbers(t *testing.T) {
	got := dedupe(t, `{"runs":[{"results":[{"stacks":[{"n":1},{"n":2},{"n":1}]}]}]}`)

	assert.Equal(t, []int{2}, stackCounts(t, got))
}

func TestDedupeStacks_RejectsInvalidReports(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"whitespace only", "  \n"},
		{"malformed JSON", "invalid json"},
		{"truncated JSON", `{"runs":[{"results":[`},
		{"trailing data", `{"runs":[]} {"runs":[]}`},
		{"array top level", `[]`},
		{"scalar top level", `"sarif"`},
		{"null top level", `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := DedupeStacks(strings.NewReader(tt.input), &out)

			require.ErrorIs(t, err, errInvalidSARIF)
			assert.Empty(t, out.String(), "nothing may be written for an invalid report")
		})
	}
}
