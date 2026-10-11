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

func repositoryURI(value string) string {
	value = strings.TrimPrefix(value, "git::")
	if strings.Contains(value, "{{") {
		return ""
	}
	if strings.HasPrefix(value, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(value, "git@"), ":", 2)
		if len(parts) == 2 {
			value = httpsScheme + parts[0] + "/" + parts[1]
		}
	}
	if !strings.Contains(value, "://") && strings.HasPrefix(value, "github.com/") {
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
		health, relevant := e.sourceHealth(ctx, o, &ref, cache)
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
	status, reason := statusPassed, ""
	if checkHasFindings(r, ruleSourceRepository) {
		status = statusFindings
	}
	if incompleteCount > 0 {
		status = "incomplete"
		reason = "Some upstream repositories could not be verified"
	}
	r.Checks = append(r.Checks, Check{ID: ruleSourceRepository, Status: status, Files: count, Reason: reason})
	return ctx.Err()
}

func (e *Engine) sourceHealth(ctx context.Context, o *Options, ref *sourceReference, cache map[string]RepositoryHealth) (RepositoryHealth, bool) {
	n := ref.node
	if n.Tag != "!!str" || strings.Contains(n.Value, "{{") || strings.HasPrefix(n.Value, "!") {
		return RepositoryHealth{repositoryInconclusive, "Dynamic repository source was not evaluated"}, true
	}
	uri := repositoryURI(n.Value)
	if !strings.Contains(uri, "://") {
		return RepositoryHealth{}, false
	}
	if health, ok := cache[uri]; ok {
		return health, true
	}
	e.emit(Event{Phase: "Check", Label: "Repository metadata"})
	health := RepositoryHealth{repositoryInconclusive, "Repository checks were disabled with --offline"}
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
	return Finding{RuleID: ruleSourceRepository, Category: "sources", Severity: validation.SeverityWarning, File: ref.document.path, Line: ref.node.Line, Column: ref.node.Column, Path: displayNodePath(ref.path), Message: health.Message, Status: status, Reason: health.Status, Remediation: "Review the upstream source and access; doctor does not rewrite source URLs"}
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
		return key == "source" || (key == "uri" && contains(path, "sources"))
	}
	// Only component source declarations; arbitrary variables named source are data.
	if key == "uri" {
		path = path[:len(path)-1]
	}
	return len(path) == 4 && path[0] == "components" && path[3] == "source"
}

func repositoryUncertain(status string) bool {
	return status == repositoryInconclusive || status == "unsupported" || status == "unavailable"
}
