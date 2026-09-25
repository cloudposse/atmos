// Package testshard partitions discovered Go tests using optional timing hints.
package testshard

import (
	"errors"
	"fmt"
	"go/token"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrPlan = errors.New("invalid test shard plan")

func validName(name string) bool {
	if name == "TestMain" || !token.IsIdentifier(name) {
		return false
	}
	for _, prefix := range []string{"Test", "Example", "Fuzz"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		if suffix == "" {
			return true
		}
		next, _ := utf8.DecodeRuneInString(suffix)
		return !unicode.IsLower(next)
	}
	return false
}

// Discover extracts executable test names from go test -list output.
func Discover(output string) ([]string, error) {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name != "TestMain" && validName(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: no tests discovered", ErrPlan)
	}
	slices.Sort(names)
	if len(slices.Compact(slices.Clone(names))) != len(names) {
		return nil, fmt.Errorf("%w: duplicate test names", ErrPlan)
	}
	return names, nil
}

// Plan assigns every discovered name exactly once, longest first. Unknown tests
// receive a one-second estimate; stale timing entries never create runnable tests.
func Plan(names []string, count int, seconds map[string]float64) ([][]string, error) {
	if count < 1 || len(names) == 0 {
		return nil, fmt.Errorf("%w: empty inventory or invalid shard count", ErrPlan)
	}
	names = slices.Clone(names)
	slices.Sort(names)
	if err := validateNames(names); err != nil {
		return nil, err
	}
	weight := func(name string) float64 { return testWeight(name, seconds) }
	slices.SortFunc(names, func(a, b string) int {
		if weight(a) > weight(b) {
			return -1
		}
		if weight(a) < weight(b) {
			return 1
		}
		return strings.Compare(a, b)
	})
	groups := make([][]string, count)
	totals := make([]float64, count)
	for _, name := range names {
		target := lightestGroup(groups, totals)
		groups[target] = append(groups[target], name)
		totals[target] += weight(name)
	}
	for _, group := range groups {
		slices.Sort(group)
	}
	return groups, nil
}

// Pattern matches complete top-level names, including all their subtests.
// An empty assignment deliberately matches nothing rather than every test.
func Pattern(names []string) string {
	if len(names) == 0 {
		return "^$"
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func testWeight(name string, seconds map[string]float64) float64 {
	value, ok := seconds[name]
	if !ok || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 1
	}
	return value
}

func validateNames(names []string) error {
	for i, name := range names {
		if !validName(name) || (i > 0 && name == names[i-1]) {
			return fmt.Errorf("%w: test name %q", ErrPlan, name)
		}
	}
	return nil
}

func lightestGroup(groups [][]string, totals []float64) int {
	target := 0
	for i := 1; i < len(groups); i++ {
		if totals[i] < totals[target] || (totals[i] == totals[target] && len(groups[i]) < len(groups[target])) {
			target = i
		}
	}
	return target
}
