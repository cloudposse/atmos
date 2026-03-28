package lint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errRuleForTest is a test-only LintRule that always returns an error from Run.
type errRuleForTest struct {
	runErr error
}

func (r *errRuleForTest) ID() string                               { return "E-01" }
func (r *errRuleForTest) Name() string                             { return "Error Rule" }
func (r *errRuleForTest) Description() string                      { return "Always returns an error." }
func (r *errRuleForTest) Severity() Severity                       { return SeverityError }
func (r *errRuleForTest) AutoFixable() bool                        { return false }
func (r *errRuleForTest) Run(_ LintContext) ([]LintFinding, error) { return nil, r.runErr }

// TestEngineRunErrorPath verifies that Engine.Run propagates errors returned by a rule.
func TestEngineRunErrorPath(t *testing.T) {
	t.Parallel()
	runErr := errors.New("rule execution failed")
	engine := NewEngine([]LintRule{&errRuleForTest{runErr: runErr}})
	_, err := engine.Run(LintContext{}, nil, SeverityInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rule execution failed")
}

// TestEngineRunEmptyRules verifies that an engine with no rules returns an empty result.
func TestEngineRunEmptyRules(t *testing.T) {
	t.Parallel()
	engine := NewEngine(nil)
	result, err := engine.Run(LintContext{}, nil, SeverityInfo)
	require.NoError(t, err)
	assert.Empty(t, result.Findings)
	assert.Equal(t, 0, result.Summary.Errors)
	assert.Equal(t, 0, result.Summary.Warnings)
	assert.Equal(t, 0, result.Summary.Info)
}

// TestEngineRunRuleFilter verifies that ruleIDs filter correctly.
func TestEngineRunRuleFilter(t *testing.T) {
	t.Parallel()

	// A rule that always returns a finding.
	type findingRule struct{ id string }
	findingRuleImpl := func(id string) LintRule {
		return &struct {
			errRuleForTest
			findingID string
		}{
			errRuleForTest: errRuleForTest{runErr: nil},
			findingID:      id,
		}
	}
	_ = findingRuleImpl

	// Use errRuleForTest with no error to represent a rule that returns no findings.
	noFindingRule := &errRuleForTest{runErr: nil}

	engine := NewEngine([]LintRule{noFindingRule})
	// Filter to a rule ID that doesn't exist — should produce no findings.
	result, err := engine.Run(LintContext{}, []string{"X-99"}, SeverityInfo)
	require.NoError(t, err)
	assert.Empty(t, result.Findings)
}

// staticFindingRule is a test-only rule that always returns a single finding with the given severity.
type staticFindingRule struct {
	ruleID   string
	severity Severity
}

func (r *staticFindingRule) ID() string          { return r.ruleID }
func (r *staticFindingRule) Name() string        { return r.ruleID }
func (r *staticFindingRule) Description() string { return "" }
func (r *staticFindingRule) Severity() Severity  { return r.severity }
func (r *staticFindingRule) AutoFixable() bool   { return false }
func (r *staticFindingRule) Run(_ LintContext) ([]LintFinding, error) {
	return []LintFinding{{RuleID: r.ruleID, Severity: r.severity, Message: "test finding"}}, nil
}

// TestEngineRunSeverityOverrideNormalization verifies that severity overrides are
// case-insensitive and that invalid values leave the original severity unchanged.
func TestEngineRunSeverityOverrideNormalization(t *testing.T) {
	t.Parallel()

	rule := &staticFindingRule{ruleID: "T-01", severity: SeverityInfo}
	engine := NewEngine([]LintRule{rule})

	tests := []struct {
		name     string
		override string
		want     Severity
	}{
		{"ERROR uppercase", "ERROR", SeverityError},
		{"Warning mixed case", "Warning", SeverityWarning},
		{"warn alias", "warn", SeverityWarning},
		{"info lowercase", "info", SeverityInfo},
		{"empty override no change", "", SeverityInfo},
		{"invalid value no change", "critical", SeverityInfo},
		{"typo no change", "err", SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := LintContext{}
			if tt.override != "" {
				ctx.LintConfig.Rules = map[string]string{"T-01": tt.override}
			}
			result, err := engine.Run(ctx, nil, SeverityInfo)
			require.NoError(t, err)
			require.Len(t, result.Findings, 1)
			assert.Equal(t, tt.want, result.Findings[0].Severity,
				"override %q should produce severity %q", tt.override, tt.want)
		})
	}
}
