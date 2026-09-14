package updater

import (
	"fmt"
	"strings"
	"text/template"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// defaultBadge is the "Atmos CI" badge used when publisher doesn't implement
// atmosgit.PullRequestBodyBadger: a plain bold link, deliberately static and dependency-free (no
// image, no forge-specific markdown assumptions) since a publisher that hasn't opted into its own
// branding shouldn't risk one that doesn't render on its own pull request markdown.
const defaultBadge = "**[Atmos CI](https://atmos.tools/ci)**"

// defaultPRBodyTemplate is the fallback `vendor.ci.pull_request.body` template, parameterized on
// the badge: a one-line explanation of what generated this PR, and the update table -- not just
// the bare table on its own, which reads as an unexplained, unbranded diff to a reviewer seeing it
// for the first time (confirmed by manual review of a real generated pull request).
const defaultPRBodyTemplate = "%s\nAutomated by `atmos vendor update --pull-request`.\n\n{{ .updates | markdownTable }}\n"

// defaultPRBody returns the fallback `vendor.ci.pull_request.body` template for publisher,
// deferring to its own atmosgit.PullRequestBodyBadger implementation (if any) for the badge.
func defaultPRBody(publisher atmosgit.PullRequestPublisher) string {
	badge := defaultBadge
	if badger, ok := publisher.(atmosgit.PullRequestBodyBadger); ok {
		badge = badger.PullRequestBodyBadge()
	}
	return fmt.Sprintf(defaultPRBodyTemplate, badge)
}

// TemplateFunctions returns the Go template function map available to pull-request
// title/body templates.
func TemplateFunctions() template.FuncMap {
	defer perf.Track(nil, "updater.TemplateFunctions")()

	return template.FuncMap{"markdownTable": func(rows []vendoring.SourceUpdateResult) string {
		var b strings.Builder
		b.WriteString("| Component | Current | Latest |\n| --- | --- | --- |\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", row.Component, row.CurrentVersion, row.LatestVersion)
		}
		return b.String()
	}}
}

// RenderPRTemplates renders the pull-request title/body from templates, falling back to the
// built-in defaults when either field is empty. publisher selects the default body's badge (see
// defaultPRBody); it has no effect when templates.Body is already set.
func RenderPRTemplates(templates PRTemplates, scope string, report *vendoring.UpdateReport, publisher atmosgit.PullRequestPublisher) (string, string, error) {
	defer perf.Track(nil, "updater.RenderPRTemplates")()

	data := map[string]any{"scope": map[string]string{"name": scope}, "updates": report.Results}
	render := func(name, value, fallback string) (string, error) {
		if value == "" {
			value = fallback
		}
		t, err := template.New(name).Funcs(TemplateFunctions()).Parse(value)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		err = t.Execute(&b, data)
		return b.String(), err
	}
	title, err := render("vendor.ci.pull_request.title", templates.Title, "chore(components): update {{ .scope.name }}")
	if err != nil {
		return "", "", err
	}
	body, err := render("vendor.ci.pull_request.body", templates.Body, defaultPRBody(publisher))
	return title, body, err
}
