package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/doctor/monorepo"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

func (e *Engine) run(ctx context.Context, o *Options) (r Report, err error) {
	r = Report{Version: 1, Findings: []Finding{}, Checks: []Check{}, ChangedFiles: []string{}, Fix: o.Fix, DryRun: o.DryRun}
	defer func() { finishReport(&r, o.Root, err) }()
	if err = validateOptions(o); err != nil {
		return r, err
	}
	if err = e.configure(o); err != nil {
		return r, err
	}
	e.emit(Event{Phase: "Discover", Label: "Project inputs"})
	invocation := *o
	o.invocation = &invocation
	p, err := e.discover(ctx, o, &r)
	if err != nil {
		return r, err
	}
	broadenAffected(o, p)
	filterDiscoveryFindings(o, p, &r)
	e.emitFindings(p.root, r.Findings)
	e.emit(Event{Phase: "Discover", Label: "Project inputs", Done: true, Status: statusPassed})
	err = e.inspect(ctx, p, o, &r)
	return r, err
}

// inspect runs every check against the discovered project and applies repairs when requested.
func (e *Engine) inspect(ctx context.Context, p *project, o *Options, r *Report) error {
	v := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})
	edits, err := e.checkDocuments(ctx, p, o, r, v)
	if err != nil {
		return err
	}
	edits, err = e.assessNaming(ctx, o, r, edits)
	if err != nil {
		return err
	}
	for i := range r.Findings {
		finding := &r.Findings[i]
		if finding.RuleID == ruleLegacyNaming {
			e.emitFindings(p.root, []Finding{*finding})
		}
	}
	for i := range r.Checks {
		if r.Checks[i].ID == ruleLegacyNaming {
			r.Checks[i] = scanCheck(r, ruleLegacyNaming, r.Checks[i].Files)
		}
	}
	before := len(r.Findings)
	checkErr := e.checkProjectValidators(ctx, p, o, r, v)
	if err = ctx.Err(); err != nil {
		return err
	}
	e.emitFindings(p.root, r.Findings[before:])
	if err = e.checkRepositories(ctx, p, o, r); err != nil {
		return err
	}
	noteUnmatchedExcludes(p, o, r)
	if checkErr != nil {
		// Each failed check is recorded on the report; repairs never proceed on a partially inspected project.
		return checkErr
	}
	if o.Fix && len(edits) > 0 {
		return e.fix(ctx, p, o, r, edits)
	}
	return nil
}

func validateOptions(o *Options) error {
	if o.DryRun && !o.Fix {
		return fmt.Errorf("%w: --dry-run requires --fix", ErrOptions)
	}
	for _, selector := range o.Only {
		found := false
		for _, rule := range Rules() {
			if selector == rule.ID || selector == rule.Category {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: unknown check %q", ErrOptions, selector)
		}
	}
	_, err := validation.ExcludePaths(nil, o.Exclude)
	return err
}

//nolint:gocritic // Progress events are value snapshots; observers must not share mutable event state.
func (e *Engine) emit(event Event) {
	if e.Observe != nil {
		e.Observe(event)
	}
}

// emitFindings sends findings to the observer with paths relative to the project root, the same
// root the per-file row labels use, so the observer can match findings to their rows.
func (e *Engine) emitFindings(root string, findings []Finding) {
	for i := range findings {
		finding := &findings[i]
		f := *finding
		if filepath.IsAbs(f.File) {
			f.File = relative(root, f.File)
		}
		e.emit(Event{Phase: "Check", Finding: &f})
	}
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// filterDiscoveryFindings drops discovery-time findings for files outside an --affected selection.
func filterDiscoveryFindings(o *Options, p *project, r *Report) {
	if !o.Affected {
		return
	}
	kept := r.Findings[:0]
	for i := range r.Findings {
		f := &r.Findings[i]
		path := f.File
		if path != "" && !filepath.IsAbs(path) {
			path = filepath.Join(p.root, filepath.FromSlash(path))
		}
		if path == "" || selected(o, p, &document{path: filepath.Clean(path)}) {
			kept = append(kept, *f)
		}
	}
	r.Findings = kept
}

func selected(o *Options, p *project, d *document) bool {
	if !o.Affected {
		return true
	}
	for _, path := range o.Selected {
		if filepath.Clean(filepath.Join(p.root, path)) == d.path {
			return true
		}
	}
	return false
}

func (e *Engine) configure(o *Options) error {
	var err error
	if o.Root == "" {
		o.Root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	o.Root, err = filepath.Abs(o.Root)
	if err != nil {
		return err
	}
	o.Root, err = filepath.EvalSymlinks(o.Root)
	if err != nil {
		return err
	}
	if err = rebaseSelected(o); err != nil {
		return err
	}
	if e.FS == nil {
		e.FS = filesystem.NewOSFileSystem()
	}
	if e.Monorepo == nil {
		e.Monorepo = monorepo.NewTreeVerifier(nil)
	}
	if e.Repositories == nil {
		e.Repositories = GitHubRepositoryChecker{}
	}
	return nil
}

// rebaseSelected converts repository-relative affected paths to paths relative to
// the canonical doctor root, so a project in a repository subdirectory still matches.
func rebaseSelected(o *Options) error {
	if o.SelectedRoot == "" {
		return nil
	}
	base, err := filepath.EvalSymlinks(o.SelectedRoot)
	if err != nil {
		return err
	}
	rebased := make([]string, 0, len(o.Selected))
	for _, path := range o.Selected {
		rel, err := filepath.Rel(o.Root, filepath.Join(base, filepath.FromSlash(path)))
		if err != nil {
			continue // Different volume; the path cannot be inside the doctor root.
		}
		rebased = append(rebased, filepath.ToSlash(rel))
	}
	o.Selected = rebased
	o.SelectedRoot = ""
	return nil
}

func broadenAffected(o *Options, p *project) {
	if o.Affected {
		// A config/stack change can affect all dependents. Discover the full closure;
		// narrow only independently verifiable workflow/vendor inputs.
		for _, path := range o.Selected {
			if validation.IsAtmosConfigPath(path) || within(p.stackDir, filepath.Join(p.root, path)) {
				o.Affected = false
				break
			}
		}
	}
}

func (e *Engine) checkDocuments(ctx context.Context, p *project, o *Options, r *Report, v validator.Validator) ([]edit, error) {
	var edits []edit
	checked := 0
	run := newMonorepoRun(p, o, r)
	for i, d := range p.docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !selected(o, p, d) {
			continue
		}
		e.emit(Event{Phase: "Check", Label: relative(p.root, d.path), Index: i})
		before := len(r.Findings)
		planned, scanErr := scanDocument(d, p, o, r, v)
		if scanErr == nil {
			var moved []edit
			moved, scanErr = e.scanMonorepo(ctx, d, run)
			planned = append(planned, moved...)
		}
		if scanErr != nil {
			return nil, scanErr
		}
		planned = e.requireDependencyValidation(p, r, planned)
		edits = append(edits, planned...)
		checked++
		status := statusPassed
		if len(r.Findings) > before {
			status = statusFindings
		}
		for j := before; j < len(r.Findings); j++ {
			finding := &r.Findings[j]
			// Naming findings are emitted after production equivalence verification.
			if finding.RuleID != ruleLegacyNaming {
				e.emitFindings(p.root, []Finding{*finding})
			}
		}
		e.emit(Event{Phase: "Check", Label: relative(p.root, d.path), Index: i + 1, Done: true, Status: status})
	}
	appendScanCoverage(o, r, checked)
	return edits, nil
}

func finishReport(r *Report, root string, err error) {
	r.SourceRoot = root
	if err != nil {
		r.OperationalError = err.Error()
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		if filepath.IsAbs(f.File) {
			f.File = relative(root, f.File)
		}
		if f.Documentation == "" {
			f.Documentation = "https://atmos.tools/cli/commands/doctor"
		}
		if f.Remediation == "" {
			f.Remediation = "Review the declaration and the rule documentation"
		}
	}
	r.sort()
}

// Run diagnoses a project using an invocation-local options snapshot.
//
//nolint:gocritic // Copy options at the public boundary so discovery cannot mutate the caller.
func (e *Engine) Run(ctx context.Context, o Options) (Report, error) {
	defer perf.Track(nil, "doctor.Engine.Run")()
	return e.run(ctx, &o)
}

func appendScanCoverage(o *Options, r *Report, checked int) {
	for _, rule := range Rules() {
		switch rule.ID {
		case ruleLegacyNaming, ruleLocalAtmosSchema, ruleComponentDependencies, ruleSchema, ruleYAMLFunctions, ruleDeprecatedField, ruleCommandProvides, ruleHookEvent, ruleTerraformReconfigure, ruleModernization, ruleComponentMonorepo, ruleImports:
			if enabled(o, rule.ID, rule.Category) {
				r.Checks = append(r.Checks, scanCheck(r, rule.ID, checked))
			}
		}
	}
}

// scanCheck builds the coverage record of a document scan. A scan that left a component source
// unverified is incomplete, not passed, so a clean exit never hides an unanswered question.
func scanCheck(r *Report, id string, files int) Check {
	status := checkStatus(r, id)
	if status == statusPassed && checkHasFindings(r, id) {
		status = statusFindings
	}
	check := Check{ID: id, Status: status, Files: files}
	if files == 0 && status == statusPassed {
		check.Status = statusSkipped
		check.Reason = "No documents were selected"
	}
	if (id == ruleComponentMonorepo || id == ruleYAMLFunctions) && hasStatus(r, id, statusVerificationIncomplete) ||
		(id == ruleYAMLFunctions && hasStatus(r, ruleImports, statusVerificationIncomplete)) {
		check.Status = "incomplete"
		check.Reason = "Some component sources could not be verified"
		if id == ruleYAMLFunctions {
			check.Reason = "Some YAML function contexts could not be inspected"
		}
	}
	if id == ruleLegacyNaming && hasStatus(r, id, statusVerificationIncomplete) {
		check.Status = "incomplete"
		check.Reason = "Some naming migrations could not be verified"
	}
	return check
}

// hasStatus reports whether any finding of the rule has the status.
func hasStatus(r *Report, id, status string) bool {
	for i := range r.Findings {
		if r.Findings[i].RuleID == id && r.Findings[i].Status == status {
			return true
		}
	}
	return false
}
