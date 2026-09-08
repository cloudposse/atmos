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
// request is recognizable as Atmos-generated at a glance, not just a bare, unexplained diff. This
// relies on raw HTML (<picture>/<source>/srcset) rendering in the pull request body, which not
// every provider's markdown supports -- see badgesByProvider for the fallback used where it's
// been confirmed not to (the raw tags render as literal text there instead of an image).
const atmosCIBadge = `<a href="https://atmos.tools/ci"><picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://atmos.tools/img/atmos-ci-gradient.svg">
  <source media="(prefers-color-scheme: light)" srcset="https://atmos.tools/img/atmos-ci-gradient-on-light.svg">
  <img src="https://atmos.tools/img/atmos-ci-gradient-on-light.svg" alt="Atmos CI" height="32" align="right">
</picture></a>
`

// atmosCIBadgePlain is atmosCIBadge's fallback for providers whose pull request markdown doesn't
// render raw HTML: a plain bold link, no image at all. An image -- raw HTML or plain markdown
// alike -- won't load anyway, since atmos.tools' CDN doesn't send the CORS headers this markdown
// renderer requires of external images (confirmed against a real pull request).
const atmosCIBadgePlain = `**[Atmos CI](https://atmos.tools/ci)**
`

// badgesByProvider maps a pull-request provider name to its markdown-appropriate Atmos CI badge.
// A provider absent from this map (including "", which schema.VendorPullRequestConfig defaults to
// its built-in provider) falls back to atmosCIBadge -- add an entry here only after confirming a
// given provider's pull request markdown doesn't render atmosCIBadge's raw HTML.
var badgesByProvider = map[string]string{
	"azuredevops": atmosCIBadgePlain,
}

// defaultPRBodyTemplate is the fallback `vendor.ci.pull_request.body` template, parameterized on
// the provider-appropriate badge: an Atmos CI badge, a one-line explanation of what generated this
// PR, and the update table -- not just the bare table on its own, which reads as an unexplained,
// unbranded diff to a reviewer seeing it for the first time (confirmed by manual review of a real
// generated pull request).
const defaultPRBodyTemplate = "%s\nAutomated by `atmos vendor update --pull-request`.\n\n{{ .updates | markdownTable }}\n"

// defaultPRBody returns the fallback `vendor.ci.pull_request.body` template for provider.
func defaultPRBody(provider string) string {
	badge, ok := badgesByProvider[provider]
	if !ok {
		badge = atmosCIBadge
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
// built-in defaults when either field is empty. provider selects the default body's badge (see
// badgesByProvider); it has no effect when templates.Body is already set.
func RenderPRTemplates(templates PRTemplates, scope string, report *vendoring.UpdateReport, provider string) (string, string, error) {
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
	body, err := render("vendor.ci.pull_request.body", templates.Body, defaultPRBody(provider))
	return title, body, err
}
