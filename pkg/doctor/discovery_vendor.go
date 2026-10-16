package doctor

import (
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/validation"
)

// vendorManifestExtensions are the extensions Atmos tries, in order, for a vendor import written
// without one. The template forms are listed last because doctor cannot read them as YAML.
var vendorManifestExtensions = []string{".yaml", ".yml", ".yaml.tmpl", ".yml.tmpl"}

// vendorImports returns the local manifests a vendor manifest lists under spec.imports. Atmos reads
// every one of them when it vendors, so doctor must check and repair them too.
func vendorImports(d *document, p *project, o *Options, r *Report) []string {
	list := nodeAt(rootNode(d.node), []string{"spec", "imports"})
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	var paths []string
	for _, item := range list.Content {
		paths = append(paths, resolveVendorImport(d, p, o, r, item)...)
	}
	return paths
}

// resolveVendorImport finds the manifest one import names. Atmos looks for the file relative to the
// working directory and then to the base path, and a manifest that imports a sibling is read from
// its own directory by the update commands, so all three places are searched in that order.
func resolveVendorImport(d *document, p *project, o *Options, r *Report, item *yaml.Node) []string {
	value := item.Value
	if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || containsTemplate(value, p.templateMarkers(d)) || hasRemoteScheme(value) {
		addNotice(o, r, incompleteNotice(d.path, "Dynamic or remote import was not evaluated"))
		return nil
	}
	for _, base := range vendorImportBases(d, p) {
		path, template := findVendorManifest(absolute(base, value))
		switch {
		case template:
			f := incomplete(d.path, "Template import not evaluated: "+relative(p.root, path))
			f.Line = item.Line
			f.Column = item.Column
			addNotice(o, r, &f)
			return nil
		case path != "":
			return []string{path}
		}
	}
	addFinding(o, r, &Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Line: item.Line, Column: item.Column, Message: "Vendor import does not resolve: " + value, Status: statusManual})
	return nil
}

// vendorImportBases lists the directories a vendor import is resolved against, without repeats.
func vendorImportBases(d *document, p *project) []string {
	var bases []string
	for _, base := range []string{p.root, p.base, filepath.Dir(d.path)} {
		if base != "" && !slices.Contains(bases, base) {
			bases = append(bases, base)
		}
	}
	return bases
}

// findVendorManifest returns the existing file for a vendor import path, and whether it is a Go
// template that doctor cannot read as YAML.
func findVendorManifest(path string) (string, bool) {
	candidates := []string{path}
	if filepath.Ext(path) == "" {
		candidates = nil
		for _, extension := range vendorManifestExtensions {
			candidates = append(candidates, path+extension)
		}
	}
	for _, candidate := range candidates {
		if exists, err := regularFile(candidate); err == nil && exists {
			return candidate, filepath.Ext(candidate) == ".tmpl"
		}
	}
	return "", false
}
