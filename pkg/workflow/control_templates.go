package workflow

import (
	"bytes"
	"strings"
	"text/template"
)

// ControlRenderFunc renders one template string against data. It lets a parallel or matrix step
// render its children with the same renderer, function set, and pass count as the context the
// parent step runs in.
type ControlRenderFunc func(name, input string, data map[string]any) (string, error)

// controlTemplates renders the template fields of parallel and matrix children. A nil value
// renders with plain text/template and no extra data.
type controlTemplates struct {
	data   ControlTemplateDataFunc
	render ControlRenderFunc
}

// newControlTemplates returns the child renderer for the supplied options.
func newControlTemplates(opts ControlExecutionOptions) *controlTemplates {
	return &controlTemplates{data: opts.TemplateData, render: opts.RenderTemplate}
}

// resolve renders input for the named child step and matrix cell. Input without a template
// delimiter is returned unchanged.
func (t *controlTemplates) resolve(input, stepName string, matrix map[string]string) (string, error) {
	if input == "" || (!strings.Contains(input, "{{") && !strings.Contains(input, "}}")) {
		return input, nil
	}
	var dataFunc ControlTemplateDataFunc
	if t != nil {
		dataFunc = t.data
	}
	data := controlTemplateData(stepName, matrix, dataFunc)
	if t != nil && t.render != nil {
		return t.render(stepName, input, data)
	}
	tmpl, err := template.New("workflow-control").Parse(input)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
