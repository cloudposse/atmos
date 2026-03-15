package lint

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// Severity represents the severity level of a lint finding.
type Severity string

const (
	// SeverityError indicates a critical issue that should block deployment.
	SeverityError Severity = "error"
	// SeverityWarning indicates a potential issue or anti-pattern.
	SeverityWarning Severity = "warning"
	// SeverityInfo indicates an informational suggestion.
	SeverityInfo Severity = "info"
)

// SeverityLevel returns a numeric value for comparison (lower = less severe).
func (s Severity) Level() int {
	switch s {
	case SeverityError:
		return 2
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 0
	default:
		return -1
	}
}

// LintFinding represents a single issue found during linting.
type LintFinding struct {
	// RuleID is the identifier of the rule that produced this finding (e.g., "L-09").
	RuleID string `json:"rule_id"`
	// Severity is the severity level of this finding.
	Severity Severity `json:"severity"`
	// Message is the human-readable description of the finding.
	Message string `json:"message"`
	// File is the path to the file where the finding was detected (may be empty).
	File string `json:"file,omitempty"`
	// Line is the line number in the file (best-effort, 0 if unknown).
	Line int `json:"line,omitempty"`
	// Component is the name of the component related to the finding (may be empty).
	Component string `json:"component,omitempty"`
	// Stack is the name of the stack related to the finding (may be empty).
	Stack string `json:"stack,omitempty"`
	// FixHint is a human-readable suggestion for fixing the finding.
	FixHint string `json:"fix_hint,omitempty"`
}

// LintConfig is an alias for schema.LintStacksConfig for use within the lint package.
type LintConfig = schema.LintStacksConfig

// LintContext holds all data available to lint rules during execution.
type LintContext struct {
	// StacksMap is the fully resolved logical stacks (post deep-merge),
	// keyed by stack name (or manifest file path).
	StacksMap map[string]any

	// RawStackConfigs contains raw physical stack manifests before merging,
	// keyed by stack manifest path.
	RawStackConfigs map[string]map[string]any

	// ImportGraph maps each file path to the list of files it imports.
	ImportGraph map[string][]string

	// StacksBasePath is the absolute path to the stacks base directory.
	StacksBasePath string

	// AllStackFiles contains all YAML files found under the stacks base path.
	AllStackFiles []string

	// AtmosConfig is the loaded Atmos configuration.
	AtmosConfig schema.AtmosConfiguration

	// LintConfig is the lint-specific configuration from atmos.yaml.
	LintConfig LintConfig
}

// LintRule is the interface that every lint rule must implement.
type LintRule interface {
	// ID returns the unique identifier of the rule (e.g., "L-09").
	ID() string
	// Name returns the human-readable name of the rule.
	Name() string
	// Description returns a longer explanation of what the rule checks.
	Description() string
	// Severity returns the default severity level of findings from this rule.
	Severity() Severity
	// AutoFixable returns true if this rule can automatically fix findings.
	AutoFixable() bool
	// Run executes the rule against the provided context and returns findings.
	Run(ctx LintContext) ([]LintFinding, error)
}

// LintResult contains the aggregate results of running the lint engine.
type LintResult struct {
	// Findings is the list of all findings from all rules.
	Findings []LintFinding `json:"findings"`
	// Summary contains counts by severity.
	Summary LintSummary `json:"summary"`
}

// LintSummary contains counts of findings by severity.
type LintSummary struct {
	// Errors is the number of error-severity findings.
	Errors int `json:"errors"`
	// Warnings is the number of warning-severity findings.
	Warnings int `json:"warnings"`
	// Info is the number of info-severity findings.
	Info int `json:"info"`
}

// HasErrors returns true if there are any error-severity findings.
func (r *LintResult) HasErrors() bool {
	return r.Summary.Errors > 0
}
