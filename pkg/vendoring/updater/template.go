package updater

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

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
	body, err := render("vendor.ci.pull_request.body", templates.Body, "{{ .updates | markdownTable }}")
	return title, body, err
}
