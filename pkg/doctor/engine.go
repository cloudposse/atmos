package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

func (e *Engine) run(ctx context.Context, o *Options) (r Report, err error) {
	r = Report{Version: 1, Findings: []Finding{}, Checks: []Check{}, ChangedFiles: []string{}, DryRun: o.DryRun}
	defer func() { finishReport(&r, o.Root, err) }()
	if err = validateOptions(o); err != nil {
		return r, err
	}
	if err = e.configure(o); err != nil {
		return r, err
	}
	e.emit(Event{Phase: "Discover", Label: "Project inputs"})
	p, err := e.discover(ctx, o, &r)
	if err != nil {
		return r, err
	}
	e.emitFindings(r.Findings)
	e.emit(Event{Phase: "Discover", Label: "Project inputs", Done: true, Status: statusPassed})
	broadenAffected(o, p)
	v := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})
	edits, err := e.checkDocuments(ctx, p, o, &r, v)
	if err != nil {
		return r, err
	}
	before := len(r.Findings)
	if err = e.checkProjectValidators(ctx, p, o, &r, v); err != nil {
		return r, err
	}
	e.emitFindings(r.Findings[before:])
	if err = e.checkRepositories(ctx, p, o, &r); err != nil {
		return r, err
	}
	if o.Fix && len(edits) > 0 {
		if err = e.fix(ctx, p, o, &r, edits); err != nil {
			return r, err
		}
	}
	return r, nil
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

func (e *Engine) emitFindings(findings []Finding) {
	for i := range findings {
		finding := &findings[i]
		f := *finding
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
	if e.FS == nil {
		e.FS = filesystem.NewOSFileSystem()
	}
	if e.Repositories == nil {
		e.Repositories = GitHubRepositoryChecker{}
	}
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
		if scanErr != nil {
			return nil, scanErr
		}
		edits = append(edits, planned...)
		checked++
		status := statusPassed
		if len(r.Findings) > before {
			status = statusFindings
		}
		e.emitFindings(r.Findings[before:])
		e.emit(Event{Phase: "Check", Label: relative(p.root, d.path), Index: i + 1, Done: true, Status: status})
	}
	appendScanCoverage(o, r, checked)
	return edits, nil
}

func finishReport(r *Report, root string, err error) {
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
		case ruleSchema, ruleDeprecatedField, ruleCommandProvides, ruleHookEvent, ruleTerraformReconfigure, ruleModernization, ruleImports:
			if enabled(o, rule.ID, rule.Category) {
				status := checkStatus(r, rule.ID)
				if status == statusPassed && checkHasFindings(r, rule.ID) {
					status = statusFindings
				}
				r.Checks = append(r.Checks, Check{ID: rule.ID, Status: status, Files: checked})
			}
		}
	}
}
