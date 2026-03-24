package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

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
	// The threshold is always set by mergedLintConfig (default 3) so we can use it
	// directly without a local fallback constant.
	threshold := ctx.LintConfig.CohesionMaxGroups
	if threshold <= 0 {
		// Safety guard: mergedLintConfig should always set a positive value, but
		// guard here for callers that construct LintContext without going through
		// the standard path (e.g. unit tests that build LintContext directly).
		threshold = 3
	}

	// Map: file -> set of concern groups (component name prefixes).
	// RawStackConfigs is keyed by file path.
	fileConcerns := make(map[string]map[string]bool)

	for filePath, rawConfig := range ctx.RawStackConfigs {
		for _, compType := range []string{cfg.TerraformSectionName, cfg.HelmfileSectionName} {
			compSection, ok := getNestedMap(rawConfig, cfg.ComponentsSectionName, compType)
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

// concernGroup returns the concern group for a component name by extracting its first
// meaningful prefix segment. The function handles the three common naming conventions
// used in Atmos component catalogs:
//
//   - Hyphen-separated:    "vpc-endpoints"   → "vpc"
//   - Underscore-separated: "vpc_endpoints"  → "vpc"
//   - Path-separated:      "network/vpc"     → "network"
//
// When the component name contains a path separator (/), the first path segment is
// used. Otherwise the first segment before a hyphen or underscore is used.
// If the name contains none of these separators it is returned unchanged.
func concernGroup(name string) string {
	// Path separator takes highest priority: "network/vpc" → "network".
	if idx := strings.IndexByte(name, '/'); idx > 0 {
		return name[:idx]
	}
	// Split on hyphen or underscore — whichever appears first.
	first := len(name)
	for _, sep := range []byte{'-', '_'} {
		if idx := strings.IndexByte(name, sep); idx > 0 && idx < first {
			first = idx
		}
	}
	if first < len(name) {
		return name[:first]
	}
	return name
}
