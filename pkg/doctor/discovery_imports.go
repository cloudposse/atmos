package doctor

import (
	"errors"
	"path/filepath"
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
		// name this import as their blocker. Stack imports never change config keys.
		p.limitKind(d.kind, d.path, mappingKeyLine(root, key, n), "Imports layer other files into this configuration")
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

// resolveLocalImport resolves one import declaration with the same resolver the stack processor
// uses, so doctor and `atmos` agree on which files an import loads.
func resolveLocalImport(d *document, p *project, o *Options, r *Report, item *yaml.Node) []string {
	item, optional := importDeclaration(item)
	if item == nil {
		return nil
	}
	value := item.Value
	if !staticImport(item, p.templateMarkers(d)) || imports.IsTemplatedPath(value) {
		addNotice(o, r, incompleteNotice(d.path, "Dynamic or remote import was not evaluated"))
		return nil
	}
	matches, err := matchLocalImport(d, p, value)
	if err != nil || len(matches) == 0 {
		if optional && (err == nil || errors.Is(err, errUtils.ErrStackImportNotFound)) {
			return nil
		}
		addFinding(o, r, &Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Line: item.Line, Message: "Local import does not resolve: " + value, Status: statusManual})
		return nil
	}
	return p.parsableImports(d, o, r, item, matches)
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
		f := incomplete(d.path, "Template import not evaluated: "+relative(p.root, match))
		f.Line = item.Line
		f.Column = item.Column
		addNotice(o, r, &f)
	}
	return parsable
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

func staticImport(item *yaml.Node, markers []string) bool {
	return item.Kind == yaml.ScalarNode && item.Tag == "!!str" && !containsTemplate(item.Value, markers) && !hasRemoteScheme(item.Value)
}
