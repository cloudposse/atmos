package render

import (
	"bytes"
	"text/template"

	"github.com/hairyhenderson/gomplate/v5/tmpl"
)

// parsePlain parses the request with text/template. When gomplate is enabled
// and the request has no Prepare hook, gomplate's `tmpl` namespace and its
// `tpl` alias are bound to the template itself, exactly as gomplate's renderer
// binds them, so every template executes on this lock-free path. The tmplPath
// argument is what `tmpl.Path` reports: gomplate's renderer passes the
// template name.
//
// The `tmpl` functions need the parsed root, so they can only be bound here,
// after the request's WrapFuncs has seen the rest of the function map. They
// are passed through WrapFuncs on their own, so policy admission covers
// `tpl` and every `tmpl.*` method like any other function.
//
// Prepare only sees the top-level parse tree; a template defined at run time
// through `tmpl.Inline` or `tpl` would escape it. Those functions are therefore
// left undefined when Prepare is set, so using them is a parse error instead
// of a way around the caller's rewrite (which is often a guard).
func parsePlain(plan *renderPlan, gomplateEnabled bool, tmplPath string) (*template.Template, error) {
	root := template.New(plan.req.Name).
		Delims(plan.left, plan.right).
		Option("missingkey=" + plan.missingKey)

	funcs := plan.funcs
	if gomplateEnabled && plan.req.Prepare == nil {
		tmplFuncs, err := plan.tmplFuncs(root, tmplPath)
		if err != nil {
			return nil, err
		}
		funcs = make(template.FuncMap, len(plan.funcs)+len(tmplFuncs))
		for k, v := range plan.funcs {
			funcs[k] = v
		}
		for k, v := range tmplFuncs {
			funcs[k] = v
		}
	}

	return root.Funcs(funcs).Parse(plan.req.Text)
}

// tmplFuncs binds gomplate's `tmpl` namespace and `tpl` alias to root and the
// request's data, then applies the request's WrapFuncs to them.
func (plan *renderPlan) tmplFuncs(root *template.Template, tmplPath string) (template.FuncMap, error) {
	ns := tmpl.New(root, plan.req.Data, tmplPath)
	funcs := template.FuncMap{
		"tmpl": func() *tmpl.Template { return ns },
		"tpl":  ns.Inline,
	}
	if plan.req.WrapFuncs == nil {
		return funcs, nil
	}
	return plan.req.WrapFuncs(funcs, plan.sources)
}

// executePlain executes a parsed template against data.
func executePlain(parsed *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
