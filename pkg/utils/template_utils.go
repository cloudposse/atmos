package utils

import (
	"bytes"
	"context"
	"os"
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
func ProcessTmpl(tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error) {
	t, err := template.New(tmplName).Funcs(sprig.FuncMap()).Parse(tmplValue)
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

// ProcessTmplWithDatasources parses and executes Go templates with datasources
func ProcessTmplWithDatasources(
	cliConfig schema.CliConfiguration,
	settingsSection schema.Settings,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	if !cliConfig.Templates.Settings.Enabled {
		return tmplValue, nil
	}

	// Merge the template settings from `atmos.yaml` CLI config and from the stack manifests
	var cliConfigTemplateSettingsMap map[any]any
	var stackManifestTemplateSettingsMap map[any]any
	var templateSettings schema.TemplatesSettings

	err := mapstructure.Decode(cliConfig.Templates.Settings, &cliConfigTemplateSettingsMap)
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(settingsSection.Templates.Settings, &stackManifestTemplateSettingsMap)
	if err != nil {
		return "", err
	}

	templateSettingsMerged, err := merge.Merge([]map[any]any{cliConfigTemplateSettingsMap, stackManifestTemplateSettingsMap})
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(templateSettingsMerged, &templateSettings)
	if err != nil {
		return "", err
	}

	// Add Gomplate and Sprig functions and datasources
	funcs := make(map[string]any)

	// Gomplate functions and datasources
	if cliConfig.Templates.Settings.Gomplate.Enabled {
		// If timeout is not provided in `atmos.yaml` nor in `settings.templates.settings` stack manifest, use 5 seconds
		timeoutSeconds, _ := lo.Coalesce(templateSettings.Gomplate.Timeout, 5)

		ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*time.Duration(timeoutSeconds))
		defer cancelFunc()

		d := data.Data{}
		d.Ctx = ctx

		for k, v := range templateSettings.Gomplate.Datasources {
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

	// Process and add environment variables
	for k, v := range templateSettings.Env {
		err = os.Setenv(k, v)
		if err != nil {
			return "", err
		}
	}

	// Process the template
	t := template.New(tmplName).Funcs(funcs)

	// Control the behavior during execution if a map is indexed with a key that is not present in the map
	// If the template context (`tmplData`) does not provide all the required variables, the following errors would be thrown:
	// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:17:12: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.flavor>: map has no entry for key "flavor"
	// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:12:36: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.stage>: map has no entry for key "stage"

	option := "missingkey=error"

	if ignoreMissingTemplateValues {
		option = "missingkey=default"
	}

	t.Option(option)

	// Number of processing steps/passes
	numSteps, _ := lo.Coalesce(templateSettings.NumSteps, 1)
	result := tmplValue

	for i := 0; i < numSteps; i++ {
		// Default delimiters
		leftDelimiter := "{{"
		rightDelimiter := "}}"

		// Check if the processing steps override the default delimiters
		if step, ok := templateSettings.Steps[i+1]; ok {
			leftDelimiter, _ = lo.Coalesce(step.LeftDelimiter, leftDelimiter)
			rightDelimiter, _ = lo.Coalesce(step.RightDelimiter, rightDelimiter)
		}

		t, err = t.Delims(leftDelimiter, rightDelimiter).Parse(result)
		if err != nil {
			return "", err
		}

		// Execute the template
		var res bytes.Buffer
		err = t.Execute(&res, tmplData)
		if err != nil {
			return "", err
		}

		result = res.String()
	}

	return result, nil
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
