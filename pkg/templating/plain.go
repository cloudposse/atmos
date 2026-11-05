package templating

import (
	"bytes"
	"text/template"

	"github.com/hairyhenderson/gomplate/v5/tmpl"
)

// parsePlain parses the request with text/template. When gomplate is enabled
// and the request has no Prepare hook, gomplate's `tmpl` namespace and its
// `tpl` alias are bound to the template itself, exactly as gomplate's renderer
// binds them, so every template executes on this lock-free path.
//
// Prepare only sees the top-level parse tree; a template defined at run time
// through `tmpl.Inline` or `tpl` would escape it. Those functions are therefore
// left undefined when Prepare is set, so using them is a parse error instead
// of a way around the caller's rewrite (which is often a guard).
func parsePlain(plan *renderPlan, gomplateEnabled bool) (*template.Template, error) {
	root := template.New(plan.req.Name).
		Delims(plan.left, plan.right).
		Option("missingkey=" + plan.missingKey)

	funcs := plan.funcs
	if gomplateEnabled && plan.req.Prepare == nil {
		funcs = make(template.FuncMap, len(plan.funcs)+len(tmplFuncNames))
		for k, v := range plan.funcs {
			funcs[k] = v
		}
		ns := tmpl.New(root, plan.req.Data, "")
		funcs["tmpl"] = func() *tmpl.Template { return ns }
		funcs["tpl"] = ns.Inline
	}

	return root.Funcs(funcs).Parse(plan.req.Text)
}

// tmplFuncNames are the names parsePlain binds to gomplate's `tmpl` namespace.
var tmplFuncNames = []string{"tmpl", "tpl"}

// executePlain executes a parsed template against data.
func executePlain(parsed *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
