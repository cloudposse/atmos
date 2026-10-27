package doctor

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/naming"
)

// limitScope says which repairs an unverifiable input blocks.
type limitScope int

const (
	// Project-wide limits block every repair: the verification itself could not be completed.
	limitProject limitScope = iota
	// Configuration limits block repairs of configuration files, whose effective values other layers can change.
	limitConfig
	// Stack limits keep the stack validators from running, because an input they read cannot be captured
	// statically. They do not block repairs of other files.
	limitStack
	// Notes record an input for later reporting. They neither block anything nor make the project incomplete.
	limitNote
)

// limit records one reason that repairs cannot be verified, with the input that causes it.
type limit struct {
	scope  limitScope
	file   string
	line   int
	reason string
}

// dynamicSite is one value that cannot be evaluated statically: a template or a custom YAML tag.
type dynamicSite struct {
	path []string
	line int
	what string
}

// limit records an unverifiable input and marks the project's inputs as incomplete.
func (p *project) limit(scope limitScope, file string, line int, reason string) {
	p.complete = false
	p.limits = append(p.limits, limit{scope: scope, file: file, line: line, reason: reason})
}

// limitKind records an unverifiable input found in a document of the given kind. Only
// configuration documents are layered; an unverifiable stack, workflow, or vendor file cannot change
// the effective value of a configuration key, so it only limits the checks that read it. Stack
// documents limit stack validation; a templated vendor source or a workflow expression must not.
func (p *project) limitKind(kind, file string, line int, reason string) {
	switch kind {
	case documentConfig:
		p.limit(limitConfig, file, line, reason)
	case documentStack:
		p.limit(limitStack, file, line, reason)
	}
}

// note records an input without limiting any check.
func (p *project) note(file string, line int, reason string) {
	p.limits = append(p.limits, limit{scope: limitNote, file: file, line: line, reason: reason})
}

// markIncomplete records that an input of this kind could not be captured. Only the stack and
// configuration documents the stack validator reads limit it; a templated vendor source or a
// workflow expression must not switch off stack validation for the whole project.
func (p *project) markIncomplete(kind string) {
	if kind == documentStack || kind == documentConfig {
		p.complete = false
	}
}

// stackBlocker names the first input that keeps stack validation from running, with its file and line
// where it has one, so a user can act on it instead of reading a generic statement about static inputs.
func (e *Engine) stackBlocker(p *project) string {
	if e.Stacks == nil {
		return "no stack validator is available"
	}
	if reason := p.recordedBlocker(); reason != "" {
		return reason
	}
	if reason := p.dynamicBlocker(); reason != "" {
		return reason
	}
	if reason := p.configLayerBlocker(); reason != "" {
		return reason
	}
	return "an input could not be captured statically"
}

// recordedBlocker describes the first limit recorded during discovery.
func (p *project) recordedBlocker() string {
	for i := range p.limits {
		if p.limits[i].scope != limitNote {
			return p.limits[i].describe(p.root)
		}
	}
	return ""
}

// dynamicBlocker describes the first template or YAML function in a stack or configuration document.
func (p *project) dynamicBlocker() string {
	for _, d := range p.docs {
		if (d.kind == documentStack || d.kind == documentConfig) && len(d.sites) > 0 {
			return d.sites[0].describe(p.root, d.path)
		}
	}
	return ""
}

// configLayerBlocker describes the configuration layering that other documents are merged under.
func (p *project) configLayerBlocker() string {
	for _, d := range p.docs {
		if d.kind == documentConfig {
			return p.layerBlocker(d)
		}
	}
	return ""
}

// describe renders the limit with the input that causes it.
func (l limit) describe(root string) string {
	if l.file == "" {
		return l.reason
	}
	location := relative(root, l.file)
	if l.line > 0 {
		location = fmt.Sprintf("%s:%d", location, l.line)
	}
	return l.reason + " (" + location + ")"
}

// describe renders the dynamic value with where it is.
func (s dynamicSite) describe(root, file string) string {
	return fmt.Sprintf("%s at %s:%d (%s) cannot be evaluated statically", s.what, relative(root, file), s.line, displayNodePath(s.path))
}

// projectLimit returns the first limit that blocks every repair, or nil.
func (p *project) projectLimit() *limit {
	for i := range p.limits {
		if p.limits[i].scope == limitProject {
			return &p.limits[i]
		}
	}
	return nil
}

// blocker explains why a candidate edit cannot be verified, or returns "" when nothing relevant
// is unverifiable. Only inputs that can change what the edit means block it: project-wide
// verification gaps, the configuration layers above or below an edited configuration file, and
// dynamic values in the subtree the edit replaces or the keys its runtime consumer reads.
func (p *project) blocker(c *edit) string {
	if l := p.projectLimit(); l != nil {
		return l.describe(p.root)
	}
	if c.doc.kind == documentConfig {
		for i := range p.limits {
			if p.limits[i].scope == limitConfig {
				return p.limits[i].describe(p.root)
			}
		}
		if reason := p.layerBlocker(c.doc); reason != "" {
			return reason
		}
	}
	for i := range c.doc.sites {
		if c.reads(c.doc.sites[i].path) {
			return c.doc.sites[i].describe(p.root, c.doc.path)
		}
	}
	return ""
}

// layerBlocker names the configuration layer that can change the effective value of an edited file.
func (p *project) layerBlocker(d *document) string {
	for _, other := range p.docs {
		if other.kind == documentConfig && other != d {
			return "Configuration is layered: " + relative(p.root, other.path) + " is also merged at runtime"
		}
	}
	if len(p.profiles) > 0 {
		return "Configuration is layered: profile " + strings.Join(p.profiles, ", ") + " is merged over " + relative(p.root, d.path)
	}
	return ""
}

// consumerKeys are the keys, relative to the edited node, that the runtime consumer of the edit reads.
func (e *edit) consumerKeys() []string {
	switch e.kind {
	case ruleCloudFormationRegion:
		return []string{"region", "settings"}
	case ruleCloudFormationTimeout:
		return []string{"timeout", "timeout_in_minutes"}
	case ruleTerraformReconfigure:
		return []string{"init_run_reconfigure", "init"}
	case ruleCommandProvides:
		return []string{"type", "semantic_type", "provides"}
	}
	return nil
}

// reads reports whether a dynamic value at site can change what the edit means: the site is the
// edited node or one of its ancestors, or lies inside a key the consumer reads.
func (e *edit) reads(site []string) bool {
	if startsWith(e.path, site) {
		return true
	}
	for _, key := range e.consumerKeys() {
		if startsWith(site, append(append([]string{}, e.path...), key)) {
			return true
		}
	}
	return false
}

// startsWith reports whether path begins with prefix.
func startsWith(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// dynamicSites lists the values that only take effect after rendering or execution: custom YAML
// tags, tag-like strings, and template expressions in any of the document's delimiters. A pure
// stacks.name_template is not one; see pureStackNameTemplate.
func dynamicSites(node *yaml.Node, markers []string) []dynamicSite {
	var sites []dynamicSite
	walk(node, nil, func(n *yaml.Node, path []string) {
		switch {
		case strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!"):
			sites = append(sites, dynamicSite{path: path, line: n.Line, what: "YAML function " + n.Tag})
		case strings.HasPrefix(n.Value, "!"):
			sites = append(sites, dynamicSite{path: path, line: n.Line, what: "YAML function " + strings.Fields(n.Value)[0]})
		case naming.Field(path) && naming.SafeTemplate(n.Value):
			// Pure naming fields are evaluated only by the production naming reporter.
		case pureStackNameTemplate(path, n.Value, markers):
			// The stack validator evaluates this template itself, with the same functions as the runtime.
		case containsTemplate(n.Value, markers):
			sites = append(sites, dynamicSite{path: path, line: n.Line, what: "Template expression"})
		}
	})
	return sites
}
