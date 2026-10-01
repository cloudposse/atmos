package managers

import (
	"bytes"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/templatefuncs"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// TemplateDelimiters returns the raw `templates.settings.delimiters` value
// from atmosConfig, or nil when the config is absent. Managers pass it
// through to RenderValueFormat so a project's custom delimiters apply to
// set-entry Format templates the same way they apply everywhere else.
func TemplateDelimiters(atmosConfig *schema.AtmosConfiguration) []string {
	defer perf.Track(atmosConfig, "managers.TemplateDelimiters")()

	if atmosConfig == nil {
		return nil
	}
	return atmosConfig.Templates.Settings.Delimiters
}

// RenderValueFormat renders a set entry's optional Format template against
// the resolved version ref, exposing .Version, .Digest, and .Pin. Shared by
// every file manager whose set entries support an optional value-reshaping
// Format field (json, yaml) so the template dialect and available functions
// stay identical across manager types. The delims argument is the raw
// `templates.settings.delimiters` value; the standard Go delimiters apply
// when it is unset or malformed.
func RenderValueFormat(formatStr string, ref manager.VersionRef, delims []string) (string, error) {
	defer perf.Track(nil, "managers.RenderValueFormat")()

	funcs := sprig.TxtFuncMap()
	delete(funcs, "env")
	delete(funcs, "expandenv")
	delete(funcs, "getHostByName")
	for name, fn := range templatefuncs.FuncMap() {
		funcs[name] = fn
	}
	left, right := tags.TemplateDelims(delims)
	tmpl, err := template.New("set-format").Delims(left, right).Funcs(funcs).Parse(formatStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ref); err != nil {
		return "", err
	}
	return buf.String(), nil
}
