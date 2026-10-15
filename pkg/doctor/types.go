// Package doctor diagnoses authored project inputs without executing configuration.
package doctor

import (
	"context"
	"errors"
	"sort"

	"github.com/cloudposse/atmos/pkg/doctor/monorepo"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

var (
	ErrOptions      = errors.New("invalid doctor options")
	ErrChanged      = errors.New("doctor source changed since inspection")
	ErrVerification = errors.New("doctor semantic verification failed")
	// ErrRecoveryManifest reports a backup manifest value that cannot be read.
	ErrRecoveryManifest = errors.New("invalid doctor recovery manifest")
	// ErrNoConfiguration reports that no Atmos configuration could be located for the project.
	ErrNoConfiguration = errors.New("no project Atmos configuration found")
)

// Rule describes a built-in check. Replacement prose is never executable.
type Rule struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// Rules is the stable, built-in check catalog.
func Rules() []Rule {
	defer perf.Track(nil, "doctor.Rules")()
	return []Rule{
		{ruleSchema, categoryValidation, "Validate authored configuration and stack schemas"},
		{ruleImports, categoryValidation, "Check local imports and report unresolved inputs"},
		{ruleStacks, categoryValidation, "Check static stack inheritance and duplicate instances"},
		{ruleDependencies, categoryValidation, "Validate static Terraform dependency graphs"},
		{ruleEditorConfig, categoryValidation, "Validate EditorConfig rules"},
		{ruleGitHubActions, categoryValidation, "Validate workflows using actionlint"},
		{ruleDeprecatedField, categoryDeprecations, "Report schema-declared deprecated fields"},
		{ruleCommandProvides, categoryDeprecations, "Migrate custom-command semantic arguments and flags"},
		{ruleHookEvent, categoryDeprecations, "Normalize recognized legacy hook events"},
		{ruleTerraformReconfigure, categoryDeprecations, "Migrate explicit Terraform reconfigure policy"},
		{ruleComponentDependencies, categoryDeprecations, "Migrate simple legacy dependencies to modern required dependencies"},
		{ruleModernization, ruleModernization, "Identify legacy Atmos GitHub Actions"},
		{ruleComponentMonorepo, ruleModernization, "Move archived terraform-aws-components sources to cloudposse-terraform-components at the same version, verified identical"},
		{ruleSourceRepository, "sources", "Check upstream repositories for archival and availability"},
	}
}

// Finding contains a diagnostic and the explicit repair decision.
type Finding struct {
	RuleID        string              `json:"rule_id"`
	Category      string              `json:"category"`
	Severity      validation.Severity `json:"severity"`
	File          string              `json:"file,omitempty"`
	Path          string              `json:"path,omitempty"`
	Line          int                 `json:"line,omitempty"`
	Column        int                 `json:"column,omitempty"`
	Message       string              `json:"message"`
	Remediation   string              `json:"remediation,omitempty"`
	Documentation string              `json:"documentation,omitempty"`
	Status        string              `json:"status"`
	Fixable       bool                `json:"fixable"`
	Reason        string              `json:"reason,omitempty"`
}

// Check records coverage separately from findings, including incomplete checks.
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Files  int    `json:"files"`
	Reason string `json:"reason,omitempty"`
}

// Report is the complete machine-readable run result.
type Report struct {
	// SourceRoot resolves project-relative locations for human source excerpts.
	SourceRoot       string    `json:"-"`
	Version          int       `json:"version"`
	Findings         []Finding `json:"findings"`
	Checks           []Check   `json:"checks"`
	ChangedFiles     []string  `json:"changed_files"`
	Backup           string    `json:"backup,omitempty"`
	Recovery         string    `json:"recovery,omitempty"`
	Fix              bool      `json:"fix"`
	DryRun           bool      `json:"dry_run"`
	OperationalError string    `json:"operational_error,omitempty"`
}

// Event communicates progress without coupling checks to terminal rendering.
type Event struct {
	Finding *Finding
	Phase   string
	Label   string
	Index   int
	Total   int
	Done    bool
	Status  string
}

// Options selects project inputs and the repair policy.
type Options struct {
	UnverifiedInputs []string
	Root             string
	ConfigFiles      []string
	ConfigDirs       []string
	Profiles         []string
	BasePath         string
	Fix              bool
	DryRun           bool
	Offline          bool
	Only             []string
	Exclude          []string
	Selected         []string
	// SelectedRoot is the absolute directory that Selected paths are relative to,
	// typically the Git worktree root. Empty means Selected is relative to Root.
	SelectedRoot string
	Affected     bool

	// invocation is the caller's options as they were before discovery moved the project root.
	// Rediscovery must resolve configuration from them, because a root derived from base_path
	// (for example the Git root) is not where the configuration was found.
	invocation *Options
}

// rediscoveryOptions returns a fresh copy of the options the first discovery started from.
func (o *Options) rediscoveryOptions() *Options {
	source := o
	if o.invocation != nil {
		source = o.invocation
	}
	clone := *source
	return &clone
}

// RepositoryChecker inspects repository metadata without cloning or installing tools.
type RepositoryChecker interface {
	Check(context.Context, string) RepositoryHealth
}

// RepositoryHealth deliberately distinguishes inaccessible repositories from missing ones.
type RepositoryHealth struct {
	Status  string
	Message string
}

// Engine dependencies are injectable to test writes and network failures.
type Engine struct {
	FS           filesystem.FileSystem
	Repositories RepositoryChecker
	BackupDir    func() (string, error)
	Observe      func(Event)
	Stacks       StackValidator
	// Monorepo proves that a component moved out of the archived monorepo has identical files.
	Monorepo monorepo.Verifier
}

// StackValidator checks decoded static inputs with the production stack consumers.
type StackValidator interface {
	Validate(context.Context, *schema.AtmosConfiguration, map[string]map[string]any) error
}

// DependencyValidator checks the production dependency graph without evaluating configuration.
type DependencyValidator interface {
	ValidateDependencies(context.Context, *schema.AtmosConfiguration, map[string]map[string]any) error
}

func (r *Report) sort() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Path < b.Path
	})
}

// ExitCode evaluates remaining findings. Operational failures cannot be suppressed, and neither
// can a requested repair that failed verification: a fix run that did not deliver what was asked
// for exits 1 whatever the severity threshold. A dry run applies nothing, so it only applies the threshold.
func (r *Report) ExitCode(threshold string) int {
	defer perf.Track(nil, "doctor.Report.ExitCode")()
	if r.OperationalError != "" {
		return 2
	}
	if r.failedRepair() {
		return 1
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == statusFixed {
			continue
		}
		if threshold != "none" && (f.Severity == validation.SeverityError || threshold == "warning" && f.Severity == validation.SeverityWarning) {
			return 1
		}
	}
	return 0
}

// failedRepair reports whether a repair that a --fix run attempted ended verification-failed.
// Without --fix nothing was requested, so a failed verification is only a finding.
func (r *Report) failedRepair() bool {
	if !r.Fix || r.DryRun {
		return false
	}
	for i := range r.Findings {
		if r.Findings[i].Status == statusVerificationFailed {
			return true
		}
	}
	return false
}

// ValidationReport adapts findings to Atmos's existing rich and SARIF renderers.
func (r *Report) ValidationReport() validation.Report {
	defer perf.Track(nil, "doctor.Report.ValidationReport")()
	v := validation.Report{}
	if r.OperationalError != "" {
		v.Diagnostics = append(v.Diagnostics, validation.Diagnostic{Source: "doctor", RuleID: "operational", Severity: validation.SeverityError, Message: r.OperationalError})
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == statusFixed {
			continue
		}
		message := f.Message
		if f.Reason != "" {
			message += ": " + f.Reason
		}
		if f.Remediation != "" {
			message += ". " + f.Remediation
		}
		v.Diagnostics = append(v.Diagnostics, validation.Diagnostic{Source: "doctor", RuleID: f.RuleID, Severity: f.Severity, File: f.File, Line: f.Line, Column: f.Column, Message: message})
	}
	return v
}

func enabled(options *Options, id, category string) bool {
	if len(options.Only) == 0 {
		return true
	}
	for _, selector := range options.Only {
		if selector == id || selector == category {
			return true
		}
	}
	return false
}
