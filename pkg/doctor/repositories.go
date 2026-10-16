package doctor

import (
	"context"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	gh "github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/validation"
)

// GitHubRepositoryChecker reuses Atmos's authenticated, host-scoped GitHub client.
type GitHubRepositoryChecker struct{}

func (GitHubRepositoryChecker) Check(ctx context.Context, uri string) RepositoryHealth {
	defer perf.Track(nil, "doctor.GitHubRepositoryChecker.Check")()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	h := gh.InspectRepository(ctx, uri)
	return RepositoryHealth{h.Status, h.Message}
}

func repositoryURI(value string, markers []string) string {
	value = strings.TrimPrefix(value, "git::")
	if containsTemplate(repositoryPart(value), markers) {
		return ""
	}
	if strings.HasPrefix(value, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(value, "git@"), ":", 2)
		if len(parts) == 2 {
			value = httpsScheme + parts[0] + "/" + parts[1]
		}
	}
	if !strings.Contains(value, schemeSeparator) && strings.HasPrefix(value, "github.com/") {
		value = httpsScheme + value
	}
	if strings.HasPrefix(value, "ssh://git@") {
		value = httpsScheme + strings.TrimPrefix(value, "ssh://git@")
	}
	owner, repo, ok := gh.ParseOwnerRepo(value)
	if !ok {
		return value
	}
	// Strip module subdirectories, ref queries, and embedded credentials from the cache key.
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if u.Port() != "" {
		host = u.Host
	}
	return httpsScheme + host + "/" + owner + "/" + repo
}

// repositoryPart returns the portion of a source that identifies the repository:
// everything before the ref query and the go-getter "//" module subdirectory.
// A template only in the ref (`?ref={{.Version}}`) or the subdirectory does not
// change which repository is checked, so such sources stay verifiable.
func repositoryPart(value string) string {
	if i := strings.Index(value, "?"); i >= 0 {
		value = value[:i]
	}
	start := 0
	if i := strings.Index(value, schemeSeparator); i >= 0 {
		start = i + len(schemeSeparator)
	}
	if i := strings.Index(value[start:], "//"); i >= 0 {
		value = value[:start+i]
	}
	return value
}

type sourceReference struct {
	document *document
	node     *yaml.Node
	path     []string
}

func sourceReferences(p *project, o *Options) []sourceReference {
	var references []sourceReference
	for _, d := range p.docs {
		if !selected(o, p, d) || (d.kind != "stack" && d.kind != "vendor") {
			continue
		}
		walk(d.node, nil, func(n *yaml.Node, path []string) {
			if len(path) == 0 || n.Kind != yaml.ScalarNode {
				return
			}
			if sourcePath(d.kind, path) {
				references = append(references, sourceReference{d, n, path})
			}
		})
	}
	return references
}

func (e *Engine) checkRepositories(ctx context.Context, p *project, o *Options, r *Report) error {
	if !enabled(o, ruleSourceRepository, "sources") {
		return nil
	}
	cache := map[string]RepositoryHealth{}
	count, incompleteCount := 0, 0
	for _, ref := range sourceReferences(p, o) {
		if err := ctx.Err(); err != nil {
			return err
		}
		health, relevant := e.sourceHealth(ctx, o, &ref, sourceCache{markers: p.templateMarkers(ref.document), health: cache})
		if !relevant {
			continue
		}
		count++
		if health.Status == "healthy" {
			continue
		}
		if repositoryUncertain(health.Status) {
			incompleteCount++
		}
		finding := repositoryFinding(&ref, health)
		r.Findings = append(r.Findings, finding)
		e.emitFindings(r.Findings[len(r.Findings)-1:])
	}
	r.Checks = append(r.Checks, repositoryCheck(r, count, incompleteCount))
	return ctx.Err()
}

// repositoryCheck summarizes the repository check. A check with nothing to inspect is skipped, not passed.
func repositoryCheck(r *Report, count, incompleteCount int) Check {
	if count == 0 && !checkHasFindings(r, ruleSourceRepository) {
		return Check{ID: ruleSourceRepository, Status: statusSkipped, Reason: "No source repository declarations"}
	}
	check := Check{ID: ruleSourceRepository, Status: statusPassed, Files: count}
	if checkHasFindings(r, ruleSourceRepository) {
		check.Status = statusFindings
	}
	if incompleteCount > 0 {
		check.Status = "incomplete"
		check.Reason = "Some upstream repositories could not be verified"
	}
	return check
}

// sourceCache shares repository results and template markers across one check phase.
type sourceCache struct {
	markers []string
	health  map[string]RepositoryHealth
}

func (e *Engine) sourceHealth(ctx context.Context, o *Options, ref *sourceReference, sc sourceCache) (RepositoryHealth, bool) {
	cache := sc.health
	n := ref.node
	if n.Tag != "!!str" || containsTemplate(repositoryPart(n.Value), sc.markers) || strings.HasPrefix(n.Value, "!") {
		return RepositoryHealth{repositoryInconclusive, "Dynamic repository source was not evaluated"}, true
	}
	uri := repositoryURI(n.Value, sc.markers)
	if !strings.Contains(uri, schemeSeparator) {
		return RepositoryHealth{}, false
	}
	if health, ok := cache[uri]; ok {
		return health, true
	}
	e.emit(Event{Phase: "Check", Label: "Repository metadata"})
	health := RepositoryHealth{repositoryInconclusive, repositoryOfflineMessage}
	if !o.Offline {
		health = e.Repositories.Check(ctx, uri)
	}
	cache[uri] = health
	outcome := statusPassed
	if health.Status != "healthy" {
		outcome = statusFindings
	}
	e.emit(Event{Phase: "Check", Label: "Repository metadata", Done: true, Status: outcome})
	return health, true
}

func repositoryFinding(ref *sourceReference, health RepositoryHealth) Finding {
	status := statusManual
	if health.Status == repositoryInconclusive || health.Status == "unsupported" {
		status = statusVerificationIncomplete
	}
	severity := validation.SeverityWarning
	if health.Message == repositoryOfflineMessage {
		// Skipping the network on request is not a problem with the project, so it never trips --fail-on=warning.
		severity = validation.SeverityNotice
	}
	return Finding{RuleID: ruleSourceRepository, Category: "sources", Severity: severity, File: ref.document.path, Line: ref.node.Line, Column: ref.node.Column, Path: displayNodePath(ref.path), Message: health.Message, Status: status, Reason: health.Status, Remediation: "Review the upstream source and access; the source-repository check does not rewrite source URLs"}
}

func checkHasFindings(r *Report, id string) bool {
	for i := range r.Findings {
		if r.Findings[i].RuleID == id {
			return true
		}
	}
	return false
}

func sourcePath(kind string, path []string) bool {
	key := path[len(path)-1]
	if kind == "vendor" {
		return key == "source" || (key == "uri" && (contains(path, "sources") || componentVendorSource(path)))
	}
	// Only component source declarations; arbitrary variables named source are data.
	if key == "uri" {
		path = path[:len(path)-1]
	}
	return len(path) == 4 && path[0] == "components" && path[3] == "source"
}

// componentVendorSource matches spec.source.uri of a component.yaml (kind ComponentVendorConfig).
func componentVendorSource(path []string) bool {
	return pathEndsWith(path, "spec", "source", "uri")
}

func repositoryUncertain(status string) bool {
	return status == repositoryInconclusive || status == "unsupported" || status == "unavailable"
}
