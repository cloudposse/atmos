package exec

import (
	"bytes"
	"context"
	"encoding/json"
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
		u.LogTrace(fmt.Sprintf("ProcessTmplWithDatasources: not processing template '%s' since templating is disabled in 'atmos.yaml'", tmplName))
		return tmplValue, nil
	}

	u.LogTrace(fmt.Sprintf("ProcessTmplWithDatasources(): processing template '%s'", tmplName))

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
		u.LogTrace(fmt.Sprintf("ProcessTmplWithDatasources(): template '%s' - evaluation %d", tmplName, i+1))

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

	u.LogTrace(fmt.Sprintf("ProcessTmplWithDatasources(): processed template '%s'", tmplName))

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

// ProcessTmplWithDatasourcesGomplate parses and executes Go templates with datasources using Gomplate
func ProcessTmplWithDatasourcesGomplate(
	atmosConfig schema.AtmosConfiguration,
	settingsSection schema.Settings,
	tmplName string,
	tmplValue string,
	mergedData map[string]interface{},
	ignoreMissingTemplateValues bool,
) (string, error) {
	// The check below is to enable ignore missing template values.
	//   If `ignoreMissingTemplateValues` is true, the missing template keys are ignored, i.e. it doesn't error out
	if ignoreMissingTemplateValues {
		os.Setenv("GOMPLATE_MISSINGKEY", "default")
		defer os.Unsetenv("GOMPLATE_MISSINGKEY")
	}
	// Step 1: Write the merged JSON data to a file
	// This file will contain the combined data that Gomplate uses to fill in the templates.
	// Gomplate reads this file directly because it's referenced by the "config" option.
	rawJSON, err := json.Marshal(mergedData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged data to JSON: %w", err)
	}

	// Create a temporary file to store JSON data merged from multiple YAML files (e.g., README.yaml).
	// This file's path is added to the "config" section of the Gomplate options, allowing Gomplate
	// to reference the data during template processing. The "." entry in the "Context" section
	// is included to maintain compatibility with an environment variable and ensure proper behavior,
	// especially on Windows.
	tmpfile, err := os.CreateTemp("", "gomplate-data-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp data file for gomplate: %w", err)
	}
	tmpName := tmpfile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpfile.Write(rawJSON); err != nil {
		tmpfile.Close()
		return "", fmt.Errorf("failed to write JSON to temp file: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp data file: %w", err)
	}

	fileURL, err := toFileScheme(tmpName)
	if err != nil {
		return "", fmt.Errorf("failed to convert temp file path to file URL: %w", err)
	}

	// fixWindowsFileURL transforms the path into a format gomplate can handle uniformly across OSes.
	finalFileUrl, err := fixWindowsFileScheme(fileURL)
	if err != nil {
		return "", err
	}

	// Step 2: Write the 'outer' top-level
	// This top-level file acts as the "outer" data source.
	// It stores a reference (README_YAML) to the 'inner' file here so Gomplate can link the environment-like data to the actual merged file.
	// Having an outer file allows us to keep a structured hierarchy of references. The "." context references this file.
	// This separation primarily deals with environment variable README_YAML.
	topLevel := map[string]interface{}{
		"Env": map[string]interface{}{
			"README_YAML": fileURL,
		},
	}
	outerJSON, err := json.Marshal(topLevel)
	if err != nil {
		return "", err
	}

	tmpfile2, err := os.CreateTemp("", "gomplate-top-level-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp data file for top-level: %w", err)
	}
	tmpName2 := tmpfile2.Name()
	defer os.Remove(tmpName2)

	if _, err = tmpfile2.Write(outerJSON); err != nil {
		tmpfile2.Close()
		return "", fmt.Errorf("failed to write top-level JSON: %w", err)
	}
	if err = tmpfile2.Close(); err != nil {
		return "", fmt.Errorf("failed to close top-level JSON: %w", err)
	}

	topLevelFileURL, err := toFileScheme(tmpName2)
	if err != nil {
		return "", fmt.Errorf("failed to convert top-level temp file path to file URL: %w", err)
	}

	// This step is crucial on Windows:
	finalTopLevelFileURL, err := fixWindowsFileScheme(topLevelFileURL)
	if err != nil {
		return "", err
	}

	// 3) Construct Gomplate Options
	//    - "." is mapped to the top-level JSON file (the 'outer' file).
	//    - "config" points to the 'inner' JSON file containing your merged data.
	//    This approach gives Gomplate two layers of data:
	//      1) outer top-level structure for environment or other references,
	//      2) inner merged data for actual template substitution.
	opts := gomplate.Options{
		Context: map[string]gomplate.Datasource{
			".": {
				URL: finalTopLevelFileURL,
			},
			"config": {
				URL: finalFileUrl,
			},
		},
		Funcs: template.FuncMap{},
	}

	// 4) Render
	renderer := gomplate.NewRenderer(opts)

	var buf bytes.Buffer
	tpl := gomplate.Template{
		Name:   tmplName,
		Text:   tmplValue,
		Writer: &buf,
	}

	if err := renderer.RenderTemplates(context.Background(), []gomplate.Template{tpl}); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return buf.String(), nil
}
