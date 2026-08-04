package updater

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// atmosCIBadge is the same responsive light/dark "Atmos CI" badge used in native CI plan/apply
// summary comments (pkg/ci/plugins/terraform), reused here so an automated component-update pull
// request is recognizable as Atmos-generated at a glance, not just a bare, unexplained diff.
const atmosCIBadge = `<a href="https://atmos.tools/ci"><picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://atmos.tools/img/atmos-ci-gradient.svg">
  <source media="(prefers-color-scheme: light)" srcset="https://atmos.tools/img/atmos-ci-gradient-on-light.svg">
  <img src="https://atmos.tools/img/atmos-ci-gradient-on-light.svg" alt="Atmos CI" height="32" align="right">
</picture></a>
`

// defaultPRBody is the fallback `vendor.ci.pull_request.body` template: the Atmos CI badge, a
// one-line explanation of what generated this PR, and the update table -- not just the bare table
// on its own, which reads as an unexplained, unbranded diff to a reviewer seeing it for the first
// time (confirmed by manual review of a real generated pull request).
const defaultPRBody = atmosCIBadge + "\nAutomated by `atmos vendor update --pull-request`.\n\n{{ .updates | markdownTable }}\n"

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
// built-in defaults when either field is empty.
func RenderPRTemplates(templates PRTemplates, scope string, report *vendoring.UpdateReport) (string, string, error) {
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
	body, err := render("vendor.ci.pull_request.body", templates.Body, defaultPRBody)
	return title, body, err
}
