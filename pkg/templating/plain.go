package templating

import (
	"bytes"
	"text/template"

	atmostemplate "github.com/cloudposse/atmos/pkg/template"
)

// parsePlain parses the request with text/template. When gomplate is enabled,
// renderer-only function names are stubbed so the template parses and can be
// inspected; when it is disabled they stay undefined, so referencing them is a
// parse error exactly as before.
func parsePlain(plan *renderPlan, gomplateEnabled bool) (*template.Template, error) {
	funcs := plan.funcs
	if gomplateEnabled {
		funcs = make(template.FuncMap, len(plan.funcs)+len(rendererOnlyFuncs))
		for k, v := range plan.funcs {
			funcs[k] = v
		}
		for k, v := range stubFuncs() {
			if _, defined := funcs[k]; !defined {
				funcs[k] = v
			}
		}
	}

	return template.New(plan.req.Name).
		Delims(plan.left, plan.right).
		Option("missingkey=" + plan.missingKey).
		Funcs(funcs).
		Parse(plan.req.Text)
}

// needsGomplateRenderer reports whether the parsed template calls anything
// that only gomplate's renderer can serve.
func needsGomplateRenderer(parsed *template.Template) bool {
	return atmostemplate.UsesFunctions(parsed, rendererOnlyFuncs, rendererOnlyMethods)
}

// executePlain executes a parsed template against data.
func executePlain(parsed *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
