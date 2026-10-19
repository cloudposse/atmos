package doctor

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/stack/imports"
	"github.com/cloudposse/atmos/pkg/validation"
)

const incompleteMessage = "Configuration verification incomplete"

// localImports returns the static local files a document imports. Template files are reported
// as unverified because they only become YAML after rendering.
func localImports(d *document, p *project, o *Options, r *Report) []string {
	var paths []string
	root := rootNode(d.node)
	for _, key := range []string{"import", ruleImports} {
		n := mappingValue(root, key)
		if n == nil {
			continue
		}
		// Imported config layers can change the effective value of any config key, so config repairs
		// name this import as their blocker. Stack imports never change config keys: the ones doctor
		// can resolve are loaded and validated, and the ones it cannot are recorded one by one.
		if d.kind == documentConfig {
			p.limit(limitConfig, d.path, mappingKeyLine(root, key, n), "Imports layer other files into this configuration")
		}
		items := n.Content
		if n.Kind == yaml.ScalarNode {
			items = []*yaml.Node{n}
		}
		for _, item := range items {
			paths = append(paths, resolveLocalImport(d, p, o, r, item)...)
		}
	}
	return paths
}

// incompleteNotice builds a verification-incomplete notice ready for addNotice.
func incompleteNotice(file, reason string) *Finding {
	f := incomplete(file, reason)
	return &f
}

func incomplete(file, reason string) Finding {
	return Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityWarning, File: file, Message: incompleteMessage, Status: statusVerificationIncomplete, Reason: reason}
}

// addFinding records a discovery-time finding only when its rule is selected, so a run limited
// with --only never reports problems from checks it was not asked to run.
func addFinding(o *Options, r *Report, f *Finding) {
	if enabled(o, f.RuleID, f.Category) {
		r.Findings = append(r.Findings, *f)
	}
}

// addNotice records a verification-incomplete notice. Notices explain why repairs and import
// closure checks are limited, so they are shown whenever imports or any repair category is selected.
func addNotice(o *Options, r *Report, f *Finding) {
	if noticeVisible(o) {
		r.Findings = append(r.Findings, *f)
	}
}

func noticeVisible(o *Options) bool {
	if enabled(o, ruleImports, categoryValidation) {
		return true
	}
	for _, rule := range Rules() {
		if rule.Category == categoryDeprecations && enabled(o, rule.ID, rule.Category) {
			return true
		}
	}
	return false
}

// dynamicImportReason explains why an import is not followed.
const dynamicImportReason = "Dynamic or remote import was not evaluated"

// resolveLocalImport resolves one import declaration with the same resolver the stack processor
// uses, so doctor and `atmos` agree on which files an import loads.
func resolveLocalImport(d *document, p *project, o *Options, r *Report, declaration *yaml.Node) []string {
	item, optional := importDeclaration(declaration)
	if item == nil {
		return nil
	}
	if declaration.Kind == yaml.MappingNode && mappingValue(declaration, "context") != nil {
		// The context only takes effect when the imported file is rendered, which doctor never does.
		p.unverifiedImport(d, o, r, item, "Import with context was not evaluated")
	}
	value := item.Value
	if !staticImport(item, p.templateMarkers(d)) || imports.IsTemplatedPath(value) {
		p.unverifiedImport(d, o, r, item, dynamicImportReason)
		return nil
	}
	matches, err := matchLocalImport(d, p, value)
	if err != nil || len(matches) == 0 {
		if !skippableImport(optional, err) {
			p.unresolvedImport(d, o, r, item)
		}
		return nil
	}
	return p.parsableImports(d, o, r, item, matches)
}

// skippableImport reports whether an import that matched nothing is optional and failed only because it is missing.
func skippableImport(optional bool, err error) bool {
	return optional && (err == nil || errors.Is(err, errUtils.ErrStackImportNotFound))
}

// unresolvedImport reports a required local import that matches no file.
func (p *project) unresolvedImport(d *document, o *Options, r *Report, item *yaml.Node) {
	message := "Local import does not resolve: " + item.Value
	p.limitKind(d.kind, d.path, item.Line, message)
	addFinding(o, r, &Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Line: item.Line, Message: message, Status: statusManual})
}

// unverifiedImport records an import that doctor cannot follow, at the line that declares it. Stack
// validation needs every file of the import closure, so it cannot run until the import is resolved.
func (p *project) unverifiedImport(d *document, o *Options, r *Report, at *yaml.Node, reason string) {
	p.limitKind(d.kind, d.path, at.Line, reason)
	f := incomplete(d.path, dynamicImportReason)
	if reason != dynamicImportReason {
		f.Reason = reason
	}
	f.Line = at.Line
	f.Column = at.Column
	addNotice(o, r, &f)
}

// matchLocalImport expands a static import path to existing files.
func matchLocalImport(d *document, p *project, value string) ([]string, error) {
	if filepath.IsAbs(value) {
		path := value
		if filepath.Ext(path) == "" {
			path += ".yaml"
		}
		return doublestar.FilepathGlob(path)
	}
	base := filepath.Dir(d.path)
	if d.kind == "stack" {
		base = p.stackDir
	}
	return imports.ResolveLocal(value, d.path, base)
}

// parsableImports drops Go template files, which cannot be read as YAML until Atmos renders
// them, and records that they were not evaluated.
func (p *project) parsableImports(d *document, o *Options, r *Report, item *yaml.Node, matches []string) []string {
	parsable := make([]string, 0, len(matches))
	for _, match := range matches {
		if !imports.IsTemplateFile(match) {
			parsable = append(parsable, match)
			continue
		}
		p.templateImport(d, o, r, item, match)
	}
	return parsable
}

// maxListedImporters bounds how many importers a template-import notice names.
const maxListedImporters = 3

// templateImport records that a document imports a Go template file doctor cannot read as YAML. The
// import closure is then incomplete, and one notice per template file names every file that imports
// it, located at the first importer, instead of one identical notice per importer.
func (p *project) templateImport(d *document, o *Options, r *Report, at *yaml.Node, target string) {
	reason := templateImportReason + relative(p.root, target)
	if d.kind == documentStack || d.kind == documentConfig {
		p.limitKind(d.kind, d.path, at.Line, reason)
	} else {
		p.note(d.path, at.Line, reason)
	}
	importers := p.templateImporters(reason)
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Message == incompleteMessage && (f.Reason == reason || strings.HasPrefix(f.Reason, reason+" (imported by")) {
			f.Reason = describeTemplateImporters(reason, p.root, importers)
			return
		}
	}
	f := incomplete(d.path, reason)
	f.Line = at.Line
	f.Column = at.Column
	addNotice(o, r, &f)
}

const templateImportReason = "Template import not evaluated: "

// templateImporters lists, in discovery order, the files recorded as importing the template named by reason.
func (p *project) templateImporters(reason string) []string {
	var files []string
	for i := range p.limits {
		if p.limits[i].reason == reason && !slices.Contains(files, p.limits[i].file) {
			files = append(files, p.limits[i].file)
		}
	}
	return files
}

// describeTemplateImporters appends the importers to the reason when more than one file imports the template.
func describeTemplateImporters(reason, root string, importers []string) string {
	if len(importers) < 2 {
		return reason
	}
	listed := make([]string, 0, maxListedImporters)
	for _, importer := range importers[:min(len(importers), maxListedImporters)] {
		listed = append(listed, relative(root, importer))
	}
	list := strings.Join(listed, ", ")
	if more := len(importers) - len(listed); more > 0 {
		list += fmt.Sprintf(", … +%d more", more)
	}
	return fmt.Sprintf("%s (imported by %d files: %s)", reason, len(importers), list)
}

func importDeclaration(item *yaml.Node) (*yaml.Node, bool) {
	if item.Kind != yaml.MappingNode {
		return item, false
	}
	optional := false
	if skip := mappingValue(item, "skip_if_missing"); skip != nil {
		_ = skip.Decode(&optional)
	}
	return mappingValue(item, "path"), optional
}

// hasRemoteScheme reports go-getter style remote import sources, which doctor never fetches.
func hasRemoteScheme(value string) bool {
	return strings.Contains(value, schemeSeparator) || strings.HasPrefix(value, "git::")
}

// staticImport reports whether an import names a local path that can be resolved without evaluating anything.
func staticImport(item *yaml.Node, markers []string) bool {
	return item.Kind == yaml.ScalarNode && item.Tag == "!!str" && !containsTemplate(item.Value, markers) && !hasRemoteScheme(item.Value) && !imports.IsRemote(item.Value)
}
