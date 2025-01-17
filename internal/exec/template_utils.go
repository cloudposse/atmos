package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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

// -----------------------------------------------------------------------------
// Existing / old code (unchanged)
// -----------------------------------------------------------------------------

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

	// Control the behavior when a map is indexed with a missing key
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

	// Merge the template settings from `atmos.yaml` + stack manifests
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

	templateSettingsMerged, err := merge.Merge(
		atmosConfig,
		[]map[string]any{cliConfigTemplateSettingsMap, stackManifestTemplateSettingsMap},
	)
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(templateSettingsMerged, &templateSettings)
	if err != nil {
		return "", err
	}

	funcs := make(map[string]any)
	evaluations, _ := lo.Coalesce(atmosConfig.Templates.Settings.Evaluations, 1)
	result := tmplValue

	for i := 0; i < evaluations; i++ {
		u.LogTrace(atmosConfig, fmt.Sprintf("ProcessTmplWithDatasources(): template '%s' - evaluation %d", tmplName, i+1))

		d := data.Data{}

		// Gomplate functions + datasources
		if atmosConfig.Templates.Settings.Gomplate.Enabled {
			timeoutSeconds, _ := lo.Coalesce(templateSettings.Gomplate.Timeout, 5)
			ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*time.Duration(timeoutSeconds))
			defer cancelFunc()

			d = data.Data{Ctx: ctx}

			for k, v := range templateSettings.Gomplate.Datasources {
				_, err := d.DefineDatasource(k, v.Url)
				if err != nil {
					return "", err
				}
				if len(v.Headers) > 0 {
					d.Sources[k].Header = v.Headers
				}
			}

			funcs = lo.Assign(funcs, gomplate.CreateFuncs(ctx, &d))
		}

		// Sprig
		if atmosConfig.Templates.Settings.Sprig.Enabled {
			funcs = lo.Assign(funcs, sprig.FuncMap())
		}

		// Atmos
		funcs = lo.Assign(funcs, FuncMap(atmosConfig, context.TODO(), &d))

		// Env from templateSettings
		for k, v := range templateSettings.Env {
			if err = os.Setenv(k, v); err != nil {
				return "", err
			}
		}

		// text/template parse
		t := template.New(tmplName).Funcs(funcs)

		leftDelimiter := "{{"
		rightDelimiter := "}}"
		if len(atmosConfig.Templates.Settings.Delimiters) > 0 {
			delimiterError := fmt.Errorf("invalid 'templates.settings.delimiters' config in 'atmos.yaml': %v\n"+
				"'delimiters' must be an array with two string items: left and right delimiter\n"+
				"the left and right delimiters must not be empty", atmosConfig.Templates.Settings.Delimiters)

			if len(atmosConfig.Templates.Settings.Delimiters) != 2 ||
				atmosConfig.Templates.Settings.Delimiters[0] == "" ||
				atmosConfig.Templates.Settings.Delimiters[1] == "" {
				return "", delimiterError
			}
			leftDelimiter = atmosConfig.Templates.Settings.Delimiters[0]
			rightDelimiter = atmosConfig.Templates.Settings.Delimiters[1]
		}
		t.Delims(leftDelimiter, rightDelimiter)

		option := "missingkey=error"
		if ignoreMissingTemplateValues {
			option = "missingkey=default"
		}
		t.Option(option)

		t, err = t.Parse(result)
		if err != nil {
			return "", err
		}

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

	/* Since there's no API method for this in Gomplate 3.11.8, have to set up env var
	 	The .Option("missingkey=default") approach isn't used to avoid having our own render logic.
		Instead we rely on standard gomplate.NewRenderer()
	*/
	if ignoreMissingTemplateValues {
		os.Setenv("GOMPLATE_MISSINGKEY", "default")
		defer os.Unsetenv("GOMPLATE_MISSINGKEY")
	}

	// Write merged data to a temporary JSON file
	// Required to make no changes to the original templates used with build-harness

	rawJSON, err := json.Marshal(mergedData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged data to JSON: %w", err)
	}

	tmpfile, err := os.CreateTemp("", "gomplate-data-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp data file for gomplate: %w", err)
	}
	tmpName := tmpfile.Name()
	defer os.Remove(tmpName)

	if _, err = tmpfile.Write(rawJSON); err != nil {
		_ = tmpfile.Close()
		return "", fmt.Errorf("failed to write JSON to temp file: %w", err)
	}
	if err = tmpfile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp data file: %w", err)
	}

	// This is the file URL, it is referenced in .Env.README_YAML
	fileURL, err := url.Parse("file://" + tmpName)
	if err != nil {
		return "", fmt.Errorf("failed to parse temp file path: %w", err)
	}

	//  Build the  top-level data object that includes Env
	topLevel := make(map[string]interface{})

	// Add .Env.README_YAML to point to fileURL
	topLevel["Env"] = map[string]interface{}{
		"README_YAML": fileURL.String(),
	}

	// Could be refactored later to avoid the second temp file, but this is more straighforward

	outerJSON, err := json.Marshal(topLevel)
	if err != nil {
		return "", fmt.Errorf("failed to marshal top-level data: %w", err)
	}

	tmpfile2, err := os.CreateTemp("", "gomplate-top-level-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp data file for top-level: %w", err)
	}
	tmpName2 := tmpfile2.Name()
	defer os.Remove(tmpName2)

	if _, err = tmpfile2.Write(outerJSON); err != nil {
		_ = tmpfile2.Close()
		return "", fmt.Errorf("failed to write top-level JSON to temp file: %w", err)
	}
	if err = tmpfile2.Close(); err != nil {
		return "", fmt.Errorf("failed to close top-level temp data file: %w", err)
	}

	topLevelFileURL, err := url.Parse("file://" + tmpName2)
	if err != nil {
		return "", fmt.Errorf("failed to parse top-level temp file path: %w", err)
	}

	// Build the gomplate Options to point the entire "dot" context at the second file
	opts := gomplate.Options{
		Context: map[string]gomplate.Datasource{
			".": {
				URL: topLevelFileURL,
			},
		},
		LDelim: "{{",
		RDelim: "}}",
		Funcs:  template.FuncMap{},
	}

	renderer := gomplate.NewRenderer(opts)

	var buf bytes.Buffer
	tpl := gomplate.Template{
		Name:   tmplName,
		Text:   tmplValue,
		Writer: &buf,
	}

	err = renderer.RenderTemplates(context.Background(), []gomplate.Template{tpl})
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return buf.String(), nil
}
