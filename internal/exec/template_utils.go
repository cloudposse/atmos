package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/mitchellh/mapstructure"
	"github.com/samber/lo"
	"gopkg.in/yaml.v2"

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

// ProcessTmplWithDatasourcesGomplate processes a template using Gomplate 3.x with configured datasources.
func ProcessTmplWithDatasourcesGomplate(
	atmosConfig schema.AtmosConfiguration,
	settingsSection schema.Settings,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	if !atmosConfig.Templates.Settings.Enabled {
		return tmplValue, nil
	}

	// Merge template settings from atmos + stack
	var cliConfigTemplateSettingsMap map[string]any
	var stackManifestTemplateSettingsMap map[string]any
	var templateSettings schema.TemplatesSettings

	if err := mapstructure.Decode(atmosConfig.Templates.Settings, &cliConfigTemplateSettingsMap); err != nil {
		return "", err
	}
	if err := mapstructure.Decode(settingsSection.Templates.Settings, &stackManifestTemplateSettingsMap); err != nil {
		return "", err
	}
	templateSettingsMerged, err := merge.Merge(
		atmosConfig,
		[]map[string]any{cliConfigTemplateSettingsMap, stackManifestTemplateSettingsMap},
	)
	if err != nil {
		return "", err
	}
	if err := mapstructure.Decode(templateSettingsMerged, &templateSettings); err != nil {
		return "", err
	}

	// Possibly multiple passes
	evaluations, _ := lo.Coalesce(atmosConfig.Templates.Settings.Evaluations, 1)
	result := tmplValue

	// Set environment variables from templateSettings.Env
	for k, v := range templateSettings.Env {
		if setErr := os.Setenv(k, v); setErr != nil {
			return "", setErr
		}
	}
	// Ensure README_YAML & README_INCLUDES exist or are "" if ignoring
	if envErr := maybeEnsureEnvVar("README_YAML", ignoreMissingTemplateValues); envErr != nil {
		return "", envErr
	}
	if envErr := maybeEnsureEnvVar("README_INCLUDES", ignoreMissingTemplateValues); envErr != nil {
		return "", envErr
	}
	// Pre-process README_YAML & README_INCLUDES to unify any multi-doc or single-element arrays
	if prepErr := multiDocUnwrap("README_YAML", ignoreMissingTemplateValues); prepErr != nil {
		return "", prepErr
	}
	if prepErr := multiDocUnwrap("README_INCLUDES", ignoreMissingTemplateValues); prepErr != nil {
		return "", prepErr
	}

	// Multi-pass loop
	for i := 0; i < evaluations; i++ {
		cfg := &gomplate.Config{
			Input:  result,
			Out:    new(bytes.Buffer),
			LDelim: "{{",
			RDelim: "}}",
		}

		// Custom delimiters
		if len(atmosConfig.Templates.Settings.Delimiters) == 2 &&
			atmosConfig.Templates.Settings.Delimiters[0] != "" &&
			atmosConfig.Templates.Settings.Delimiters[1] != "" {
			cfg.LDelim = atmosConfig.Templates.Settings.Delimiters[0]
			cfg.RDelim = atmosConfig.Templates.Settings.Delimiters[1]
		}

		// If gomplate is enabled, define datasources from templateSettings
		if atmosConfig.Templates.Settings.Gomplate.Enabled {
			for dsName, dsInfo := range templateSettings.Gomplate.Datasources {
				cfg.DataSources = append(cfg.DataSources, dsName+"="+dsInfo.Url)
			}
		}

		if err := gomplate.RunTemplates(cfg); err != nil {
			return "", fmt.Errorf("gomplate pass %d for '%s': %w", i+1, tmplName, err)
		}

		// Grab the rendered output
		buf := cfg.Out.(*bytes.Buffer)
		result = buf.String()

		// Parse it as YAML
		anyData, parseErr := u.UnmarshalYAML[any](result)
		if parseErr != nil {
			return "", fmt.Errorf("yaml parse error pass %d: %w", i+1, parseErr)
		}

		// If it’s an array with length 1, unwrap it
		if arr, ok := anyData.([]interface{}); ok && len(arr) == 1 {
			anyData = arr[0]
		}
		dataMap, ok := anyData.(map[string]any)
		if !ok {
			return "", fmt.Errorf("yaml parse pass %d: expected object but got %T", i+1, anyData)
		}

		// Update templateSettings from the newly rendered YAML
		if resultMapSettings, ok := dataMap["settings"].(map[string]any); ok {
			if resultMapSettingsTemplates, ok := resultMapSettings["templates"].(map[string]any); ok {
				if resultMapSettingsTemplatesSettings, ok := resultMapSettingsTemplates["settings"].(map[string]any); ok {
					if decErr := mapstructure.Decode(resultMapSettingsTemplatesSettings, &templateSettings); decErr != nil {
						return "", decErr
					}
				}
			}
		}
	}
	return result, nil
}

// maybeEnsureEnvVar sets env var 'name' to "" if missing
func maybeEnsureEnvVar(name string, ignore bool) error {
	if _, found := os.LookupEnv(name); !found {
		if ignore {
			_ = os.Setenv(name, "")
		} else {
			return fmt.Errorf("environment variable %s is missing; set it or enable ignoreMissingTemplateValues", name)
		}
	}
	return nil
}

// multiDocUnwrap ensures the file ref'd by envVarName is loaded as multi-document YAML
func multiDocUnwrap(envVarName string, ignore bool) error {
	path, found := os.LookupEnv(envVarName)
	if !found || path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if ignore {
			return nil
		}
		return fmt.Errorf("file %q for %s not found; set it or enable ignoreMissingTemplateValues", path, envVarName)
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}

	// read the raw file
	raw, err := os.ReadFile(path)
	if err != nil {
		if ignore {
			return nil
		}
		return fmt.Errorf("error reading %q for %s: %w", path, envVarName, err)
	}

	// parse as multi-doc
	docs, docErr := parseMultiDocYAML(raw)
	if docErr != nil {
		if ignore {
			return nil
		}
		return fmt.Errorf("invalid multi-doc YAML in %q for %s: %w", path, envVarName, docErr)
	}
	if len(docs) == 0 {
		// no docs => do nothing
		return nil
	}
	// If exactly 1 doc, keep that doc
	var single interface{}
	if len(docs) == 1 {
		single = docs[0]
	} else {
		single = docs[0]
	}

	if arr, ok := single.([]interface{}); ok && len(arr) == 1 {
		single = arr[0]
	}

	_, isMap := single.(map[string]any)
	if isMap {
		newYAML, serErr := u.ConvertToYAML(single)
		if serErr != nil {
			if ignore {
				return nil
			}
			return fmt.Errorf("unable to serialize unwrapped data for %s: %w", envVarName, serErr)
		}

		tmp, tmpErr := os.CreateTemp("", envVarName+"-*.yaml")
		if tmpErr != nil {
			if ignore {
				return nil
			}
			return fmt.Errorf("failed to create temp for %s: %w", envVarName, tmpErr)
		}
		defer tmp.Close()

		if _, writeErr := tmp.Write([]byte(newYAML)); writeErr != nil {
			if ignore {
				return nil
			}
			return fmt.Errorf("failed to write unwrapped data for %s: %w", envVarName, writeErr)
		}

		if setErr := os.Setenv(envVarName, tmp.Name()); setErr != nil {
			if ignore {
				return nil
			}
			return fmt.Errorf("failed to reset %s to new file: %w", envVarName, setErr)
		}
	}
	return nil
}

// parseMultiDocYAML tries to parse `raw` as multi-document YAML, returns a slice of docs.
func parseMultiDocYAML(raw []byte) ([]interface{}, error) {
	var docs []interface{}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	for {
		var doc interface{}
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
