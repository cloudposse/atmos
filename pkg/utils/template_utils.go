package utils

import (
	"bytes"
	"context"
	"text/template"
	"text/template/parse"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessTmpl parses and executes Go templates
func ProcessTmpl(
	cliConfig schema.CliConfiguration,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	if !cliConfig.Templates.Settings.Enabled {
		return tmplValue, nil
	}

	// Add Gomplate and Sprig functions
	funcs := make(map[string]any)

	if cliConfig.Templates.Settings.Gomplate.Enabled {
		// Process and add Gomplate `datasources`
		d := data.Data{}
		for k, v := range cliConfig.Templates.Settings.Gomplate.Datasources {
			_, err := d.DefineDatasource(k, v)
			if err != nil {
				return "", err
			}
		}

		funcs = lo.Assign(funcs, gomplate.CreateFuncs(context.Background(), &d))
	}
	if cliConfig.Templates.Settings.Sprig.Enabled {
		funcs = lo.Assign(funcs, sprig.FuncMap())
	}

	t, err := template.New(tmplName).Funcs(funcs).Parse(tmplValue)
	if err != nil {
		return "", err
	}

	// Control the behavior during execution if a map is indexed with a key that is not present in the map
	// If the template context (`tmplData`) does not provide all the required variables, the following errors would be thrown:
	// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:17:12: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.flavor>: map has no entry for key "flavor"
	// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:12:36: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.stage>: map has no entry for key "stage"

	option := "missingkey=error"

	if ignoreMissingTemplateValues {
		option = "missingkey=default"
	}

	t.Option(option)

	var res bytes.Buffer
	err = t.Execute(&res, tmplData)
	if err != nil {
		return "", err
	}

	return res.String(), nil
}

// IsGolangTemplate checks if the provided string is a Go template
func IsGolangTemplate(str string) (bool, error) {
	t, err := template.New(str).Parse(str)
	if err != nil {
		return false, err
	}

	isGoTemplate := false

	// Iterate over all nodes in the template and check if any of them is of type `NodeAction` (field evaluation)
	for _, node := range t.Root.Nodes {
		if node.Type() == parse.NodeAction {
			isGoTemplate = true
			break
		}
	}

	return isGoTemplate, nil
}
