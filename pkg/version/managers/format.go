package managers

import (
	"bytes"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/templatefuncs"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// RenderValueFormat renders a set entry's optional Format template against
// the resolved version ref, exposing .Version, .Digest, and .Pin. Shared by
// every file manager whose set entries support an optional value-reshaping
// Format field (json, yaml) so the template dialect and available functions
// stay identical across manager types.
func RenderValueFormat(formatStr string, ref manager.VersionRef) (string, error) {
	defer perf.Track(nil, "managers.RenderValueFormat")()

	funcs := sprig.TxtFuncMap()
	delete(funcs, "env")
	delete(funcs, "expandenv")
	delete(funcs, "getHostByName")
	for name, fn := range templatefuncs.FuncMap() {
		funcs[name] = fn
	}
	tmpl, err := template.New("set-format").Funcs(funcs).Parse(formatStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ref); err != nil {
		return "", err
	}
	return buf.String(), nil
}
