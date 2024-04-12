package utils

import (
	"bytes"
	"context"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/mitchellh/mapstructure"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessTmpl parses and executes Go templates
func ProcessTmpl(
	cliConfig schema.CliConfiguration,
	settingsTemplates schema.SettingsTemplates,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	if !cliConfig.Templates.Settings.Enabled {
		return tmplValue, nil
	}

	// Add Gomplate and Sprig functions and datasources
	funcs := make(map[string]any)

	// Gomplate functions and datasources
	if cliConfig.Templates.Settings.Gomplate.Enabled {
		// Merge the datasources from `atmos.yaml` and from the `settings.templates` section in stack manifests
		var cliConfigDatasources map[any]any
		var stackManifestDatasources map[any]any

		err := mapstructure.Decode(cliConfig.Templates.Settings.Gomplate.Datasources, &cliConfigDatasources)
		if err != nil {
			return "", err
		}

		err = mapstructure.Decode(settingsTemplates.Gomplate.Datasources, &stackManifestDatasources)
		if err != nil {
			return "", err
		}

		merged, err := merge.Merge([]map[any]any{cliConfigDatasources, stackManifestDatasources})
		if err != nil {
			return "", err
		}

		var datasources map[string]schema.TemplatesSettingsGomplateDatasource
		err = mapstructure.Decode(merged, &datasources)
		if err != nil {
			return "", err
		}

		// If timeout is not provided, use 5 seconds
		timeoutSeconds := cliConfig.Templates.Settings.Gomplate.Timeout
		if timeoutSeconds == 0 {
			timeoutSeconds = 5
		}

		ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*time.Duration(timeoutSeconds))
		defer cancelFunc()

		d := data.Data{}
		d.Ctx = ctx

		for k, v := range datasources {
			_, err := d.DefineDatasource(k, v.Url)
			if err != nil {
				return "", err
			}

			// Add datasource headers
			if len(v.Headers) > 0 {
				d.Sources[k].Header = v.Headers
			}
		}

		funcs = lo.Assign(funcs, gomplate.CreateFuncs(ctx, &d))
	}

	// Sprig functions
	if cliConfig.Templates.Settings.Sprig.Enabled {
		funcs = lo.Assign(funcs, sprig.FuncMap())
	}

	// Process the template
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

	// Execute the template
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
