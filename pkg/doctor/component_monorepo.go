package doctor

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/doctor/monorepo"
	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	monorepoMessage     = "Component source uses the archived terraform-aws-components monorepo"
	monorepoOfflineNote = "Repository verification was skipped with --offline"
	monorepoRetryNote   = "Run atmos doctor --fix again without --offline and with access to GitHub"
	keyVersion          = "version"
	keySource           = "source"
	keyURI              = "uri"
)

// refPattern matches a single Git tag or branch name. Version ranges, placeholders such as
// "CUSTOM, > 1.422", and anything else with spaces or operators do not name one revision.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+-]*$`)

// ErrMonorepoVersion reports a source whose version cannot be checked against a single tag.
var ErrMonorepoVersion = errors.New("component version cannot be resolved to a tag")

// monorepoRun holds the state shared by every document of one run: verifications are looked up
// once per component and ref, however many sources use them.
type monorepoRun struct {
	p        *project
	o        *Options
	r        *Report
	verified map[string]monorepo.Verification
}

func newMonorepoRun(p *project, o *Options, r *Report) *monorepoRun {
	return &monorepoRun{p: p, o: o, r: r, verified: map[string]monorepo.Verification{}}
}

// siteKind says what a monorepo reference is used for, which decides whether doctor can repair it.
type siteKind int

const (
	siteSource siteKind = iota
	siteMixin
	siteOther
)

// scanMonorepo finds the sources of a document that point at the archived monorepo, reports each
// one, and returns the repairs for those whose successor repository was verified to hold identical files.
func (e *Engine) scanMonorepo(ctx context.Context, d *document, run *monorepoRun) ([]edit, error) {
	if d.node == nil || !enabled(run.o, ruleComponentMonorepo, ruleModernization) {
		return nil, nil
	}
	var edits []edit
	seen := map[*yaml.Node]bool{}
	var failure error
	walk(d.node, nil, func(n *yaml.Node, path []string) {
		site := monorepoSiteAt(d, n, path)
		if failure != nil || site == nil || seen[n] {
			return
		}
		seen[n] = true
		if failure = ctx.Err(); failure != nil {
			return
		}
		if planned := e.planMonorepoSite(ctx, run, site); planned != nil {
			edits = append(edits, *planned)
		}
	})
	return edits, failure
}

// monorepoSite is one reference to the archived monorepo that doctor reports.
type monorepoSite struct {
	doc  *document
	node *yaml.Node
	path []string
	kind siteKind
}

// monorepoSiteAt returns the site for a node that names the monorepo where doctor looks for
// sources, or nil for every other node.
func monorepoSiteAt(d *document, n *yaml.Node, path []string) *monorepoSite {
	if len(path) == 0 || n.Kind != yaml.ScalarNode || n.Tag != "!!str" || !monorepo.Mentions(n.Value) {
		return nil
	}
	kind, relevant := classifySite(d.kind, path)
	if !relevant {
		return nil
	}
	return &monorepoSite{doc: d, node: n, path: path, kind: kind}
}

// classifySite decides whether a scalar at path is a monorepo reference doctor reports, and how.
func classifySite(documentKind string, path []string) (siteKind, bool) {
	switch {
	case mixinURI(path):
		return siteMixin, true
	case sourceLocation(documentKind, path):
		return siteSource, true
	case componentSourceKey(path):
		return siteOther, true
	}
	return siteOther, false
}

// sourceLocation matches the declarations that name a component's upstream: spec.source.uri of a
// component.yaml, spec.sources[].source of a vendor manifest, and a component's source or
// source.uri in a stack.
func sourceLocation(documentKind string, path []string) bool {
	switch documentKind {
	case documentVendor:
		return componentVendorSource(path) || (len(path) == 4 && path[0] == "spec" && path[1] == "sources" && path[3] == keySource)
	case "stack":
		return sourcePath(documentKind, path)
	}
	return false
}

// planMonorepoSite reports one reference and returns the repair when one is available.
func (e *Engine) planMonorepoSite(ctx context.Context, run *monorepoRun, site *monorepoSite) *edit {
	byHand := "Change the source to the component's repository in cloudposse-terraform-components by hand: " + monorepo.MigrationGuideURL
	reference, ok := monorepo.Parse(site.node.Value)
	switch {
	case site.kind == siteMixin:
		run.add(site, statusManual, "Mixin is fetched from the archived terraform-aws-components monorepo", "Mixins are reviewed by hand; automatic repair covers component sources", "Point the mixin at a maintained location: "+monorepo.MigrationGuideURL)
		return nil
	case site.kind == siteOther:
		run.add(site, statusManual, monorepoMessage, "This value is not a component source declaration, so it is reviewed by hand", byHand)
		return nil
	case !ok:
		run.add(site, statusManual, monorepoMessage, "The source is written in a form doctor cannot rewrite", byHand)
		return nil
	}
	result, err := reference.Rewrite()
	if err != nil {
		run.addUnmovable(site, &reference, err)
		return nil
	}
	if reason := run.p.monorepoBlocker(site.doc, site.path); reason != "" {
		run.add(site, statusVerificationIncomplete, monorepoMessage, "Not fixed automatically because "+reason, "Change the source to "+result.Source+" by hand")
		return nil
	}
	ref, err := monorepoRef(site.doc, site.path, &result)
	if err != nil {
		run.add(site, statusManual, monorepoMessage, err.Error(), "Pin the version to one tag of the monorepo, then run atmos doctor --fix")
		return nil
	}
	return run.conclude(site, &result, e.verifyMonorepo(ctx, run, &result, ref))
}

// add records a finding for the site.
func (run *monorepoRun) add(s *monorepoSite, status, message, reason, remediation string) int {
	run.r.Findings = append(run.r.Findings, Finding{
		RuleID: ruleComponentMonorepo, Category: ruleModernization, Severity: validation.SeverityWarning,
		File: s.doc.path, Path: displayNodePath(s.path), Line: s.node.Line, Column: s.node.Column,
		Message: message, Reason: reason, Remediation: remediation, Status: status,
	})
	return len(run.r.Findings) - 1
}

// addUnmovable reports a reference that has no automatic successor, with what to do about it.
func (run *monorepoRun) addUnmovable(s *monorepoSite, reference *monorepo.Reference, err error) {
	module := "modules/" + reference.Module
	switch {
	case errors.Is(err, monorepo.ErrSubdirInQuery):
		corrected, _ := reference.QuerySubdirCorrection()
		run.add(s, statusManual, monorepoMessage,
			"The subdirectory is inside the query string; go-getter requires the //subdirectory before the ?, so this source does not select that directory",
			"Change the source to "+corrected+" (the subdirectory goes before the query)")
	case errors.Is(err, monorepo.ErrSplitComponent):
		run.add(s, statusManual, fmt.Sprintf("Component %s was split into separate repositories", module),
			"Its subcomponents moved to cloudposse-terraform-components/"+strings.Join(monorepo.Successors(reference.Module), ", "),
			"Follow the special migration procedure for split components: "+monorepo.MigrationGuideURL)
	case errors.Is(err, monorepo.ErrNoSuccessor):
		run.add(s, statusManual, fmt.Sprintf("Component %s has no successor repository", module),
			"It was removed from the monorepo before the migration to cloudposse-terraform-components",
			"Keep the pinned monorepo tag, or replace the component with a maintained alternative")
	case errors.Is(err, monorepo.ErrTemplatedModule):
		run.add(s, statusManual, monorepoMessage, "The module path is a template that doctor cannot evaluate",
			"Change the source to the component's repository in cloudposse-terraform-components by hand: "+monorepo.MigrationGuideURL)
	default:
		run.add(s, statusManual, "Source selects no module of the archived terraform-aws-components monorepo", err.Error(),
			"Change the source to the component's repository in cloudposse-terraform-components by hand: "+monorepo.MigrationGuideURL)
	}
}

// conclude turns a verification into the finding and, when the move is proven, the repair.
func (run *monorepoRun) conclude(s *monorepoSite, result *monorepo.Result, v monorepo.Verification) *edit {
	switch v.Verdict {
	case monorepo.VerdictIdentical:
		index := run.add(s, "available", monorepoMessage, v.Reason, "Moves to "+result.Slug()+" at the same version (files verified identical)")
		run.r.Findings[index].Fixable = true
		return &edit{doc: s.doc, path: s.path, kind: ruleComponentMonorepo, value: result.Source, from: s.node.Value, finding: index}
	case monorepo.VerdictDiffers:
		run.add(s, statusVerificationFailed, monorepoMessage, v.Reason, "Pin a version that exists in both repositories, or review the differences and change the source to "+result.Source+" by hand")
	default:
		run.add(s, statusVerificationIncomplete, monorepoMessage, v.Reason, monorepoRetryNote)
	}
	return nil
}

// verifyMonorepo compares the module with its successor at ref, once per component and ref.
func (e *Engine) verifyMonorepo(ctx context.Context, run *monorepoRun, result *monorepo.Result, ref string) monorepo.Verification {
	key := result.Module + "\x00" + ref
	if v, ok := run.verified[key]; ok {
		return v
	}
	v := monorepo.Verification{Verdict: monorepo.VerdictUnverified, Reason: monorepoOfflineNote}
	if !run.o.Offline {
		e.emit(Event{Phase: "Check", Label: "Component repository " + result.Slug()})
		v = e.Monorepo.Verify(ctx, *result, ref)
		e.emit(Event{Phase: "Check", Label: "Component repository " + result.Slug(), Done: true, Status: verdictStatus(v.Verdict)})
	}
	run.verified[key] = v
	return v
}

// verdictStatus maps a verdict to the progress status of the check that produced it.
func verdictStatus(v monorepo.Verdict) string {
	if v == monorepo.VerdictIdentical {
		return statusPassed
	}
	return statusFindings
}

// monorepoRef returns the single revision a source fetches: its literal ref, or the version field
// that fills the {{.Version}} placeholder. Anything that is not one tag cannot be verified.
func monorepoRef(d *document, path []string, result *monorepo.Result) (string, error) {
	ref := result.Ref
	switch result.RefKind {
	case monorepo.RefNone:
		return "", fmt.Errorf("%w: the source pins no ref, so it follows the default branch", ErrMonorepoVersion)
	case monorepo.RefDynamic:
		return "", fmt.Errorf("%w: the ref %s is a template that doctor cannot evaluate", ErrMonorepoVersion, ref)
	case monorepo.RefVersion:
		version, err := declaredVersion(d, path)
		if err != nil {
			return "", err
		}
		ref = version
	}
	if !refPattern.MatchString(ref) {
		return "", fmt.Errorf("%w: %q is not a single tag", ErrMonorepoVersion, ref)
	}
	return ref, nil
}

// declaredVersion reads the version that fills {{.Version}}: the sibling of the source in a
// vendor manifest or a component's source mapping. A source written as a plain string in a stack
// has no version beside it.
func declaredVersion(d *document, path []string) (string, error) {
	last := path[len(path)-1]
	if last == keySource && d.kind != documentVendor {
		return "", fmt.Errorf("%w: the source uses the {{.Version}} placeholder but declares no version", ErrMonorepoVersion)
	}
	versionPath := append(append([]string{}, path[:len(path)-1]...), keyVersion)
	node := nodeAt(rootNode(d.node), versionPath)
	if node == nil || node.Kind != yaml.ScalarNode || node.Value == "" {
		return "", fmt.Errorf("%w: the source uses the {{.Version}} placeholder but declares no version next to it", ErrMonorepoVersion)
	}
	if strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!") || strings.Contains(node.Value, "{{") {
		return "", fmt.Errorf("%w: the version is dynamic", ErrMonorepoVersion)
	}
	return strings.TrimSpace(node.Value), nil
}

// monorepoBlocker explains why a source in the document cannot be verified, or returns "". A
// template in the source itself is the expected {{.Version}} placeholder, so only the nodes
// that enclose the source count.
func (p *project) monorepoBlocker(d *document, path []string) string {
	if l := p.projectLimit(); l != nil {
		return l.describe(p.root)
	}
	for i := range d.sites {
		if site := d.sites[i]; len(site.path) < len(path) && startsWith(path, site.path) {
			return site.describe(p.root, d.path)
		}
	}
	return ""
}

// normalizeMonorepoSource accepts exactly the source change the rule makes. The node must hold
// the original source or its rewrite, and the rewrite must be the one the rewriter derives from
// the original, so no other source change passes verification.
func normalizeMonorepoSource(node *yaml.Node, e *edit) error {
	want, ok := monorepo.Rewrite(e.from)
	if !ok || want.Source != e.value {
		return fmt.Errorf("%w: the recorded source change is not the monorepo move", ErrVerification)
	}
	if node.Kind != yaml.ScalarNode || (node.Value != e.from && node.Value != e.value) {
		return fmt.Errorf("%w: the source changed beyond the repository move", ErrVerification)
	}
	node.Value = e.value
	return nil
}

// mixinURI matches the uri of an entry in a component.yaml mixins list.
func mixinURI(path []string) bool {
	return len(path) >= 4 && path[len(path)-1] == keyURI && path[len(path)-3] == "mixins"
}
