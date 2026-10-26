package workflow

import (
	"bytes"
	"text/template"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
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
	delims delimiters.Delimiters
}

// newControlTemplates returns the child renderer for the supplied options.
func newControlTemplates(opts ControlExecutionOptions) *controlTemplates {
	return &controlTemplates{data: opts.TemplateData, render: opts.RenderTemplate, delims: opts.Delimiters.Normalize()}
}

// resolve renders input for the named child step and matrix cell. Input without a template
// delimiter is returned unchanged.
func (t *controlTemplates) resolve(input, stepName string, matrix map[string]string) (string, error) {
	var dataFunc ControlTemplateDataFunc
	delims := delimiters.Default()
	if t != nil {
		dataFunc = t.data
		delims = t.delims
	}
	if input == "" || !delims.ContainsAny(input) {
		return input, nil
	}
	data := controlTemplateData(stepName, matrix, dataFunc)
	if t != nil && t.render != nil {
		return t.render(stepName, input, data)
	}
	tmpl, err := delims.Apply(template.New("workflow-control")).Parse(input)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// fieldError reports a failed template render of a child step field, with the `!literal` hint
// naming the delimiters the children render with.
func (t *controlTemplates) fieldError(step *schema.WorkflowStep, field string, cause error) error {
	delims := delimiters.Default()
	if t != nil {
		delims = t.delims
	}
	return stepPkg.TemplateFieldErrorWithDelimiters(delims, step, field, cause)
}
