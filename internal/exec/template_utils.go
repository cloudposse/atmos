package exec

import (
	"bytes"
	"context"
	"fmt"
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
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ProcessTmpl parses and executes Go templates
func ProcessTmpl(
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	d := data.Data{}
	ctx := context.TODO()

	// Add Gomplate, Sprig and Atmos template functions
	funcs := lo.Assign(gomplate.CreateFuncs(ctx, &d), sprig.FuncMap(), FuncMap(schema.AtmosConfiguration{}, ctx, &d))

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

// ProcessTmplWithDatasources parses and executes Go templates with datasources
func ProcessTmplWithDatasources(
	atmosConfig schema.AtmosConfiguration,
	settingsSection schema.Settings,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	if !atmosConfig.Templates.Settings.Enabled {
		u.LogTrace(atmosConfig, fmt.Sprintf("ProcessTmplWithDatasources: not processing template '%s' since templating is disabled in 'atmos.yaml'", tmplName))
		return tmplValue, nil
	}

	u.LogTrace(atmosConfig, fmt.Sprintf("ProcessTmplWithDatasources(): processing template '%s'", tmplName))

	// Merge the template settings from `atmos.yaml` CLI config and from the stack manifests
	var cliConfigTemplateSettingsMap map[string]any
	var stackManifestTemplateSettingsMap map[string]any
	var templateSettings schema.TemplatesSettings

	err := mapstructure.Decode(atmosConfig.Templates.Settings, &cliConfigTemplateSettingsMap)
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(settingsSection.Templates.Settings, &stackManifestTemplateSettingsMap)
	if err != nil {
		return "", err
	}

	templateSettingsMerged, err := merge.Merge(atmosConfig, []map[string]any{cliConfigTemplateSettingsMap, stackManifestTemplateSettingsMap})
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(templateSettingsMerged, &templateSettings)
	if err != nil {
		return "", err
	}

	// Add Atmos, Gomplate and Sprig functions and datasources
	funcs := make(map[string]any)

	// Number of processing evaluations/passes
	evaluations, _ := lo.Coalesce(atmosConfig.Templates.Settings.Evaluations, 1)
	result := tmplValue

	for i := 0; i < evaluations; i++ {
		u.LogTrace(atmosConfig, fmt.Sprintf("ProcessTmplWithDatasources(): template '%s' - evaluation %d", tmplName, i+1))

		d := data.Data{}

		// Gomplate functions and datasources
		if atmosConfig.Templates.Settings.Gomplate.Enabled {
			// If timeout is not provided in `atmos.yaml` nor in `settings.templates.settings` stack manifest, use 5 seconds
			timeoutSeconds, _ := lo.Coalesce(templateSettings.Gomplate.Timeout, 5)

			ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*time.Duration(timeoutSeconds))
			defer cancelFunc()

			d = data.Data{}
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
		if atmosConfig.Templates.Settings.Sprig.Enabled {
			funcs = lo.Assign(funcs, sprig.FuncMap())
		}

		// Atmos functions
		funcs = lo.Assign(funcs, FuncMap(atmosConfig, context.TODO(), &d))

		// Process and add environment variables
		for k, v := range templateSettings.Env {
			err = os.Setenv(k, v)
			if err != nil {
				return "", err
			}
		}

		// Process the template
		t := template.New(tmplName).Funcs(funcs)

		// Template delimiters
		leftDelimiter := "{{"
		rightDelimiter := "}}"

		if len(atmosConfig.Templates.Settings.Delimiters) > 0 {
			delimiterError := fmt.Errorf("invalid 'templates.settings.delimiters' config in 'atmos.yaml': %v\n"+
				"'delimiters' must be an array with two string items: left and right delimiter\n"+
				"the left and right delimiters must not be an empty string", atmosConfig.Templates.Settings.Delimiters)

			if len(atmosConfig.Templates.Settings.Delimiters) != 2 {
				return "", delimiterError
			}

			if atmosConfig.Templates.Settings.Delimiters[0] == "" {
				return "", delimiterError
			}

			if atmosConfig.Templates.Settings.Delimiters[1] == "" {
				return "", delimiterError
			}

			leftDelimiter = atmosConfig.Templates.Settings.Delimiters[0]
			rightDelimiter = atmosConfig.Templates.Settings.Delimiters[1]
		}

		t.Delims(leftDelimiter, rightDelimiter)

		// Control the behavior during execution if a map is indexed with a key that is not present in the map
		// If the template context (`tmplData`) does not provide all the required variables, the following errors would be thrown:
		// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:17:12: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.flavor>: map has no entry for key "flavor"
		// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:12:36: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.stage>: map has no entry for key "stage"

		option := "missingkey=error"

		if ignoreMissingTemplateValues {
			option = "missingkey=default"
		}

		t.Option(option)

		// Parse the template
		t, err = t.Parse(result)
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
		resultMap, err := u.UnmarshalYAML[schema.AtmosSectionMapType](result)
		if err != nil {
			return "", err
		}

		if resultMapSettings, ok := resultMap["settings"].(map[string]any); ok {
			if resultMapSettingsTemplates, ok := resultMapSettings["templates"].(map[string]any); ok {
				if resultMapSettingsTemplatesSettings, ok := resultMapSettingsTemplates["settings"].(map[string]any); ok {
					err = mapstructure.Decode(resultMapSettingsTemplatesSettings, &templateSettings)
					if err != nil {
						return "", err
					}
				}
			}
		}
	}

	u.LogTrace(atmosConfig, fmt.Sprintf("ProcessTmplWithDatasources(): processed template '%s'", tmplName))

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
