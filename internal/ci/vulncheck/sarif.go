// Package vulncheck runs govulncheck for CI and normalizes its SARIF report so
// GitHub code scanning accepts it. It backs the `go tool mage ci:vulncheck`
// target used by .github/workflows/codeql.yml.
package vulncheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	runsKey    = "runs"
	resultsKey = "results"
	stacksKey  = "stacks"
)

var errInvalidSARIF = errors.New("invalid SARIF report")

// DedupeStacks copies a SARIF report from r to w, removing exact duplicate
// objects from every result's `stacks` array. The scanner can emit identical
// stacks, but the SARIF schema requires the entries to be unique, so GitHub
// rejects the upload otherwise.
//
// Only complete duplicates are removed: the first occurrence of each stack
// keeps its position, frame order and metadata, and findings, distinct stacks
// and all other properties pass through unchanged. Objects are compared
// independently of key order. Absent, null or non-array `stacks` values, and
// absent or null `runs`/`results`, are left as they are.
//
// Malformed or empty input is an error, so a broken scan can never be replaced
// by an empty report.
func DedupeStacks(r io.Reader, w io.Writer) error {
	defer perf.Track(nil, "vulncheck.DedupeStacks")()

	decoder := json.NewDecoder(r)
	// Keep numbers as written instead of round-tripping them through float64.
	decoder.UseNumber()

	var report any
	if err := decoder.Decode(&report); err != nil {
		return fmt.Errorf("%w: %w", errInvalidSARIF, err)
	}
	if decoder.More() {
		return fmt.Errorf("%w: unexpected data after the top-level value", errInvalidSARIF)
	}
	root, ok := report.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: top-level value must be an object", errInvalidSARIF)
	}

	if err := dedupeRuns(root[runsKey]); err != nil {
		return err
	}

	encoder := json.NewEncoder(w)
	// Stack messages contain characters such as < and & that must stay readable.
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(root)
}

// dedupeRuns edits each run's results in place. A missing or non-array value
// is not an error: it carries nothing to deduplicate.
func dedupeRuns(runs any) error {
	list, ok := runs.([]any)
	if !ok {
		return nil
	}
	for _, run := range list {
		runObject, ok := run.(map[string]any)
		if !ok {
			continue
		}
		results, ok := runObject[resultsKey].([]any)
		if !ok {
			continue
		}
		for _, result := range results {
			if err := dedupeResultStacks(result); err != nil {
				return err
			}
		}
	}
	return nil
}

func dedupeResultStacks(result any) error {
	resultObject, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	stacks, ok := resultObject[stacksKey].([]any)
	if !ok {
		return nil
	}
	unique, err := uniqueByContent(stacks)
	if err != nil {
		return err
	}
	resultObject[stacksKey] = unique
	return nil
}

// uniqueByContent keeps the first occurrence of each element. Elements compare
// by their canonical JSON encoding, which sorts object keys.
func uniqueByContent(items []any) ([]any, error) {
	seen := make(map[string]struct{}, len(items))
	unique := make([]any, 0, len(items))
	for _, item := range items {
		key, err := canonicalJSON(item)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, item)
	}
	return unique, nil
}

func canonicalJSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", fmt.Errorf("%w: %w", errInvalidSARIF, err)
	}
	return buffer.String(), nil
}
