package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/lint"
)

const defaultCohesionThreshold = 3

// l05CohesionRule flags catalog files that define components spanning more than a
// configurable number of inferred concern groups (based on component name prefixes).
type l05CohesionRule struct{}

func newL05CohesionRule() lint.LintRule {
	return &l05CohesionRule{}
}

func (r *l05CohesionRule) ID() string   { return "L-05" }
func (r *l05CohesionRule) Name() string { return "Catalog File Cohesion" }
func (r *l05CohesionRule) Description() string {
	return "Flags catalog files that define components whose names span more than the configured number of concern groups."
}
func (r *l05CohesionRule) Severity() lint.Severity { return lint.SeverityInfo }
func (r *l05CohesionRule) AutoFixable() bool       { return false }

func (r *l05CohesionRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	threshold := defaultCohesionThreshold

	// Map: file -> set of concern groups (component name prefixes).
	// RawStackConfigs is keyed by file path.
	fileConcerns := make(map[string]map[string]bool)

	for filePath, rawConfig := range ctx.RawStackConfigs {
		for _, compType := range []string{"terraform", "helmfile"} {
			compSection, ok := getNestedMap(rawConfig, "components", compType)
			if !ok {
				continue
			}
			for compName := range compSection {
				group := concernGroup(compName)
				if _, ok := fileConcerns[filePath]; !ok {
					fileConcerns[filePath] = make(map[string]bool)
				}
				fileConcerns[filePath][group] = true
			}
		}
	}

	var findings []lint.LintFinding
	for file, concerns := range fileConcerns {
		if len(concerns) > threshold {
			groups := make([]string, 0, len(concerns))
			for g := range concerns {
				groups = append(groups, g)
			}
			displayPath := file
			if ctx.StacksBasePath != "" {
				if rel, err := filepath.Rel(ctx.StacksBasePath, file); err == nil {
					displayPath = rel
				}
			}
			findings = append(findings, lint.LintFinding{
				RuleID:   r.ID(),
				Severity: r.Severity(),
				File:     displayPath,
				Message:  fmt.Sprintf("Catalog file '%s' spans %d concern groups (%s), exceeding the threshold of %d", displayPath, len(concerns), strings.Join(groups, ", "), threshold),
				FixHint:  "Split this catalog file into separate files, one per concern group",
			})
		}
	}

	return findings, nil
}

// concernGroup returns the concern group for a component name by extracting its prefix.
func concernGroup(name string) string {
	// Use the first segment of a hyphen-separated name or path segment.
	parts := strings.SplitN(name, "-", 2)
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return name
}

// getNestedMap retrieves a nested map by key path.
func getNestedMap(m map[string]any, keys ...string) (map[string]any, bool) {
	current := m
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
