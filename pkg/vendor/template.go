package vendor

import (
	"bytes"
	"context"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v3"
)

// processVendorTemplate expands the component/version templates accepted by
// vendor manifests without coupling this package to the command executor.
func processVendorTemplate(name, value string, data any) (string, error) {
	t, err := template.New(name).
		Funcs(sprig.FuncMap()).
		Funcs(gomplate.CreateFuncs(context.Background(), nil)).
		Funcs(template.FuncMap{"env": os.Getenv}).
		Option("missingkey=error").
		Parse(value)
	if err != nil {
		return "", err
	}

	var output bytes.Buffer
	if err := t.Execute(&output, data); err != nil {
		return "", err
	}
	return output.String(), nil
}
