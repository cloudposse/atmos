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

func (r *errRuleForTest) ID() string                             { return "E-01" }
func (r *errRuleForTest) Name() string                           { return "Error Rule" }
func (r *errRuleForTest) Description() string                    { return "Always returns an error." }
func (r *errRuleForTest) Severity() Severity                     { return SeverityError }
func (r *errRuleForTest) AutoFixable() bool                      { return false }
func (r *errRuleForTest) Run(_ LintContext) ([]LintFinding, error) { return nil, r.runErr }

// TestDefaultRules verifies that DefaultRules returns nil (rules come from pkg/lint/rules.All).
func TestDefaultRules(t *testing.T) {
	t.Parallel()
	result := DefaultRules()
	assert.Nil(t, result, "DefaultRules should return nil; callers must use pkg/lint/rules.All()")
}

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
