package report

import (
	"fmt"
	"sort"
	"strings"
)

// orderEntries sorts entries into report order: by scheduled position, then by arrival. Entries
// without a position follow every positioned one, in arrival order, so a sequential run is ordered
// by when its steps ran and a parallel run by where its branches were scheduled.
func orderEntries(entries []*pending) []*Entry {
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if c := comparePositions(left.entry.Position, right.entry.Position); c != 0 {
			return c < 0
		}
		return left.arrival < right.arrival
	})
	ordered := make([]*Entry, len(entries))
	for index, item := range entries {
		ordered[index] = item.entry
	}
	return ordered
}

// comparePositions compares two scheduled positions lexicographically. A nil position is greater
// than any non-nil one; a shorter position that is a prefix of a longer one is smaller.
func comparePositions(left, right []int) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return 1
	case right == nil:
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			if left[index] < right[index] {
				return -1
			}
			return 1
		}
	}
	return len(left) - len(right)
}

// assemble builds the report context and the section contexts for the ordered entries.
func assemble(entries []*Entry) (*Report, []*Operation) {
	report := &Report{Heading: reportHeading, Total: len(entries)}
	operations := make([]*Operation, 0, len(entries))
	for index, entry := range entries {
		operation := buildOperation(index+1, entry)
		operations = append(operations, operation)
		report.Rows = append(report.Rows, rowOf(operation))
		count(report, operation)
	}
	report.Overview = overview(report)
	return report, operations
}

// count adds one operation to the totals. An unchanged operation is a success that wrote nothing,
// and is counted once, as unchanged.
func count(report *Report, operation *Operation) {
	switch operation.Kind {
	case kindFailed:
		report.Failed++
	case kindCanceled:
		report.Canceled++
	case kindUnchanged:
		report.Unchanged++
	default:
		report.Succeeded++
	}
}

// overview words the totals: "3 operations: 1 succeeded, 1 unchanged, 1 failed".
func overview(report *Report) string {
	noun := "operations"
	if report.Total == 1 {
		noun = "operation"
	}
	parts := make([]string, 0, 4)
	for _, item := range []struct {
		count int
		word  string
	}{
		{report.Succeeded, "succeeded"},
		{report.Unchanged, "unchanged"},
		{report.Failed, "failed"},
		{report.Canceled, "canceled"},
	} {
		if item.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", item.count, item.word))
		}
	}
	return fmt.Sprintf("%d %s: %s.", report.Total, noun, strings.Join(parts, ", "))
}

// rowOf builds the overview table row of an operation. Code spans are made table-safe by escaping
// pipes, which a table cell would otherwise split on.
func rowOf(operation *Operation) Row {
	artifactText := operation.Ref
	if artifactText == "" {
		artifactText = operation.Requested
	}
	if artifactText == "" && operation.Collection != nil {
		artifactText = fmt.Sprintf("%d artifacts", operation.Collection.Count)
	}
	row := Row{
		Number:      operation.Number,
		Operation:   operation.ActionLabel,
		Icon:        operation.Icon,
		Status:      operation.Status,
		Artifact:    cellSafe(artifactText),
		Repository:  cellSafe(operation.Repository),
		Destination: cellSafe(operation.Destination),
		Size:        operation.Size,
		Duration:    operation.Duration,
	}
	if len(operation.Digests) > 0 {
		primary := operation.Digests[0]
		row.Digest = primary.Short + " (" + strings.ToLower(primary.Label) + ")"
	}
	return row
}

// cellSafe escapes the pipes of an already-built code span so it stays inside one table cell.
func cellSafe(span string) string {
	return strings.ReplaceAll(span, "|", `\|`)
}
