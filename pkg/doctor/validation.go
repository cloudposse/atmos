package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	ecconfig "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	ecvalidation "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/validation"

	civalidate "github.com/cloudposse/atmos/pkg/ci/validate"
	"github.com/cloudposse/atmos/pkg/ci/validate/githubactions"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

// projectCheck is one project-wide validator. Its label names the progress event to close if it fails.
type projectCheck struct {
	id    string
	label string
	run   func() error
}

// checkProjectValidators runs every project-wide validator. An operational failure in one
// validator is recorded on that check and the remaining validators still run; the joined error
// keeps the overall result operational.
func (e *Engine) checkProjectValidators(ctx context.Context, p *project, o *Options, r *Report, v validator.Validator) error {
	checks := []projectCheck{
		{ruleDependencies, "Dependencies", func() error { return e.checkDependencies(ctx, p, o, r) }},
		{ruleStacks, "Static stacks", func() error { return e.checkStacks(ctx, p, o, r) }},
		{ruleGitHubActions, "GitHub Actions", func() error { return e.checkActions(ctx, p, o, r) }},
		{ruleEditorConfig, "EditorConfig", func() error { return e.checkEditorConfig(p, o, r) }},
		{"schema/custom", "", func() error { return e.customSchemas(ctx, p, o, r, v) }},
	}
	var failures []error
	for _, check := range checks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := check.run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e.recordCheckFailure(r, &check, err)
			failures = append(failures, fmt.Errorf("%s: %w", check.id, err))
		}
	}
	return errors.Join(failures...)
}

// recordCheckFailure marks a check as failed by an operational error, replacing any partial result.
func (e *Engine) recordCheckFailure(r *Report, check *projectCheck, err error) {
	failed := Check{ID: check.id, Status: statusFailed, Reason: err.Error()}
	replaced := false
	for i := range r.Checks {
		if r.Checks[i].ID == check.id {
			r.Checks[i] = failed
			replaced = true
			break
		}
	}
	if !replaced {
		r.Checks = append(r.Checks, failed)
	}
	if check.label != "" {
		e.emit(Event{Phase: "Check", Label: check.label, Done: true, Status: statusFailed})
	}
}

func (e *Engine) checkActions(ctx context.Context, p *project, o *Options, r *Report) error {
	if !enabled(o, ruleGitHubActions, categoryValidation) {
		return nil
	}
	var paths []string
	for _, d := range p.docs {
		if d.kind == "workflow" && selected(o, p, d) {
			paths = append(paths, d.path)
		}
	}
	if len(paths) == 0 {
		r.Checks = append(r.Checks, Check{ID: ruleGitHubActions, Status: "skipped", Reason: "No selected workflow files"})
		return nil
	}
	e.emit(Event{Phase: "Check", Label: "GitHub Actions"})
	report, err := (githubactions.Validator{}).Validate(ctx, civalidate.Request{Root: p.root, Paths: paths, ConfigFile: actionlintConfig(p.root)})
	if err != nil {
		return err
	}
	appendActionDiagnostics(r, report)
	status := checkStatus(r, ruleGitHubActions)
	r.Checks = append(r.Checks, Check{ID: ruleGitHubActions, Status: status, Files: len(paths)})
	e.emit(Event{Phase: "Check", Label: "GitHub Actions", Done: true, Status: status})
	return nil
}

func (e *Engine) checkEditorConfig(p *project, o *Options, r *Report) error {
	if !enabled(o, ruleEditorConfig, categoryValidation) {
		return nil
	}
	pathsToConfig, available, err := editorConfigPaths(p)
	if err != nil {
		return err
	}
	if !available {
		r.Checks = append(r.Checks, Check{ID: ruleEditorConfig, Status: "skipped", Reason: "No EditorConfig configuration"})
		return nil
	}
	e.emit(Event{Phase: "Check", Label: "EditorConfig"})
	config := ecconfig.NewConfig(pathsToConfig)
	if err := config.Parse(); err != nil {
		return err
	}
	// validate.editorconfig in atmos.yaml shapes the run exactly as it does for
	// "atmos validate editorconfig", and files come from the same Git-aware selection.
	settings, err := validation.EditorConfigSettingsFromMap(p.config)
	if err != nil {
		return err
	}
	validation.ApplyEditorConfigSettings(config, &settings)
	paths, err := validation.EditorConfigFiles(p.root, config, settings.Exclude)
	if err != nil {
		return err
	}
	paths, err = selectedPaths(p, o, paths)
	if err != nil {
		return err
	}
	for _, fileErrors := range ecvalidation.ProcessValidation(paths, *config) {
		for _, issue := range fileErrors.Errors {
			r.Findings = append(r.Findings, Finding{RuleID: ruleEditorConfig, Category: categoryValidation, Severity: validation.SeverityError, File: fileErrors.FilePath, Line: max(0, issue.LineNumber), Message: issue.Message.Error(), Status: statusManual})
		}
	}
	status := checkStatus(r, ruleEditorConfig)
	r.Checks = append(r.Checks, Check{ID: ruleEditorConfig, Status: status, Files: len(paths)})
	e.emit(Event{Phase: "Check", Label: "EditorConfig", Done: true, Status: status})
	return nil
}

func selectedPaths(p *project, o *Options, paths []string) ([]string, error) {
	var result []string
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if !within(p.root, abs) {
			continue
		}
		// Every selected-or-not path counts toward the --exclude patterns that match it.
		excluded, err := p.excludes(o, relative(p.root, abs))
		if err != nil {
			return nil, err
		}
		if !excluded && selected(o, p, &document{path: abs}) {
			result = append(result, abs)
		}
	}
	return result, nil
}

func (e *Engine) customSchemas(ctx context.Context, p *project, o *Options, r *Report, v validator.Validator) error {
	if !enabled(o, ruleSchema, categoryValidation) && !enabled(o, ruleDeprecatedField, categoryDeprecations) {
		return nil
	}
	registries, _ := p.config["schemas"].(map[string]any)
	for key, value := range registries {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if err := e.customSchema(ctx, &schemaRun{p, o, r, v}, key, entry); err != nil {
			return err
		}
	}
	return nil
}

// schemaRun carries the shared inputs of the custom schema checks.
type schemaRun struct {
	p *project
	o *Options
	r *Report
	v validator.Validator
}

// customSchema validates the files one schema registry entry matches.
func (e *Engine) customSchema(ctx context.Context, run *schemaRun, key string, entry map[string]any) error {
	p, o, r, v := run.p, run.o, run.r, run.v
	source, _ := entry[ruleSchema].(string)
	if source == "" {
		return nil
	}
	source, reason, err := e.captureSchema(p, source)
	if err != nil {
		return err
	}
	if reason != "" {
		p.limit(limitProject, "", 0, "schema "+key+": "+reason)
		r.Checks = append(r.Checks, Check{ID: "schema/" + key, Status: "incomplete", Reason: reason})
		return nil
	}
	paths, err := schemaMatchPaths(p, o, entry)
	if err != nil {
		return err
	}
	before := len(r.Findings)
	if err = e.checkSchemaFiles(ctx, p, o, r, schemaCheck{v, source, paths}); err != nil {
		return err
	}
	// Each schema check reports only the findings it added.
	added := Report{Findings: r.Findings[before:]}
	status := checkStatus(&added, ruleSchema)
	if len(paths) == 0 {
		status = statusSkipped
	}
	r.Checks = append(r.Checks, Check{ID: "schema/" + key, Status: status, Files: len(paths)})
	return nil
}

func schemaMatchPaths(p *project, o *Options, entry map[string]any) ([]string, error) {
	matches, _ := entry["matches"].([]any)
	seen := map[string]bool{}
	var result []string
	for _, pattern := range matches {
		glob, ok := pattern.(string)
		if !ok {
			continue
		}
		if !filepath.IsAbs(glob) {
			glob = filepath.Join(p.root, glob)
		}
		paths, err := doublestar.FilepathGlob(glob)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			if !seen[path] {
				result = append(result, path)
				seen[path] = true
			}
		}
	}
	return selectedPaths(p, o, result)
}

func (e *Engine) checkSchemaFiles(ctx context.Context, p *project, o *Options, r *Report, check schemaCheck) error {
	v, source, paths := check.validator, check.source, check.paths
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, err := e.FS.ReadFile(path)
		if err != nil {
			return err
		}
		if p.byPath[path] == nil {
			p.extra[path] = content
		}
		node, parseErr := parse(content)
		if parseErr != nil {
			r.Findings = append(r.Findings, *parseFinding(path, parseErr))
			continue
		}
		d := &document{path: path, content: content, node: node, schema: source}
		if err = scanSchema(d, o, r, v); err != nil {
			return err
		}
	}
	return nil
}

func appendDiagnostics(r *Report, report validation.Report) {
	for _, d := range report.Diagnostics {
		r.Findings = append(r.Findings, Finding{RuleID: d.Source, Category: categoryValidation, Severity: d.Severity, File: d.File, Line: d.Line, Column: d.Column, Message: d.Message, Status: statusManual})
	}
}

func checkStatus(r *Report, id string) string {
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.RuleID == id && f.Severity == validation.SeverityError {
			return "failed"
		}
	}
	return statusPassed
}

type schemaCheck struct {
	validator validator.Validator
	source    string
	paths     []string
}

// captureSchema bounds schema resolution to a captured local document or bundled schema.
// External references require a transitive source snapshot before they can justify repairs.
func (e *Engine) captureSchema(p *project, source string) (string, string, error) {
	if containsTemplate(source, []string{tags.DefaultLeftDelim}) || strings.HasPrefix(source, "!") {
		return source, "Dynamic schema source was not evaluated", nil
	}
	if strings.HasPrefix(source, "atmos://") {
		return source, "", nil
	}
	if strings.Contains(source, schemeSeparator) {
		// Remote schemas are mutable inputs, and may themselves fetch executable or
		// remote references. Diagnosis and repairs report this coverage gap explicitly.
		return source, "Remote schema cannot be included in the static source snapshot", nil
	}
	source = absolute(p.root, source)
	content, err := e.FS.ReadFile(source)
	if err != nil {
		return source, "", err
	}
	p.extra[source] = content
	var document any
	if err := json.Unmarshal(content, &document); err != nil {
		return source, "", err
	}
	if externalSchemaReference(document) {
		return source, "External schema references need a transitive source snapshot; validation was not attempted", nil
	}
	return source, "", nil
}

func externalSchemaReference(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if referenceKeyword(key) && isExternalReference(child) {
				return true
			}
			if externalSchemaReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if externalSchemaReference(child) {
				return true
			}
		}
	}
	return false
}

func isExternalReference(value any) bool {
	ref, ok := value.(string)
	return ok && !strings.HasPrefix(ref, "#")
}

// editorConfigPaths returns the existing EditorConfig Checker configuration files, in
// precedence order, and whether any EditorConfig input exists. Absent defaults are never passed
// to the checker, which treats a missing file as an error rather than as no configuration.
func editorConfigPaths(p *project) ([]string, bool, error) {
	candidates := []string{filepath.Join(p.root, ".editorconfig-checker.json"), filepath.Join(p.root, ".ecrc")}
	settings, _ := p.config["validate"].(map[string]any)
	editor, _ := settings[ruleEditorConfig].(map[string]any)
	configured, _ := editor["config_file_paths"].([]any)
	for _, value := range configured {
		if path, ok := value.(string); ok {
			candidates = append(candidates, absolute(p.root, path))
		}
	}
	var paths []string
	for _, path := range candidates {
		exists, err := regularFile(path)
		if err != nil {
			return nil, false, err
		}
		if exists {
			paths = append(paths, path)
		}
	}
	available := len(paths) > 0
	if !available {
		exists, err := regularFile(filepath.Join(p.root, ".editorconfig"))
		if err != nil {
			return nil, false, err
		}
		available = exists
	}
	return paths, available, nil
}

func regularFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}

func referenceKeyword(key string) bool {
	return key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef"
}
