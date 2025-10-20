package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"
	_ "github.com/hairyhenderson/gomplate/v4"
	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Cache for sprig function maps to avoid repeated expensive allocations.
// Sprig function maps are immutable once created, so caching is safe.
// Note: Gomplate functions are NOT cached as they may have state and context dependencies.
var (
	sprigFuncMapCache     template.FuncMap
	sprigFuncMapCacheOnce sync.Once
)

// getSprigFuncMap returns a cached copy of the sprig function map.
// Sprig function maps are expensive to create (173MB+ allocations) and immutable,
// so we cache and reuse them across template operations.
// This optimization reduces heap allocations by ~3.76% (173MB) per profile run.
func getSprigFuncMap() template.FuncMap {
	sprigFuncMapCacheOnce.Do(func() {
		sprigFuncMapCache = sprig.FuncMap()
	})
	return sprigFuncMapCache
}

// ProcessTmpl parses and executes Go templates.
func ProcessTmpl(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ProcessTmpl")()

	d := data.Data{}
	ctx := context.TODO()

	// Add Gomplate, Sprig and Atmos template functions.
	cfg := atmosConfig
	if cfg == nil {
		cfg = &schema.AtmosConfiguration{}
	}
	funcs := lo.Assign(gomplate.CreateFuncs(ctx, &d), getSprigFuncMap(), FuncMap(cfg, &schema.ConfigAndStacksInfo{}, ctx, &d))

	t, err := template.New(tmplName).Funcs(funcs).Parse(tmplValue)
	if err != nil {
		return "", err
	}

	// Control the behavior during execution if a map is indexed with a key that is not present in the map.
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

// ProcessTmplWithDatasources parses and executes Go templates with datasources.
func ProcessTmplWithDatasources(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	settingsSection schema.Settings,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ProcessTmplWithDatasources")()

	if !atmosConfig.Templates.Settings.Enabled {
		log.Debug("ProcessTmplWithDatasources: not processing templates since templating is disabled in 'atmos.yaml'", "template", tmplName)
		return tmplValue, nil
	}

	log.Debug("ProcessTmplWithDatasources", "template", tmplName)

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
		log.Debug("ProcessTmplWithDatasources", "template", tmplName, "evaluation", i+1)

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
			funcs = lo.Assign(funcs, getSprigFuncMap())
		}

		// Atmos functions
		funcs = lo.Assign(funcs, FuncMap(atmosConfig, configAndStacksInfo, context.TODO(), &d))

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

	log.Debug("ProcessTmplWithDatasources: processed", "template", tmplName)

	return result, nil
}

// IsGolangTemplate checks if the provided string is a Go template.
func IsGolangTemplate(atmosConfig *schema.AtmosConfiguration, str string) (bool, error) {
	defer perf.Track(atmosConfig, "exec.IsGolangTemplate")()
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

// Create temporary directory.
func createTempDirectory() (string, error) {
	// Create a temporary directory for the temporary files.
	tempDir, err := os.MkdirTemp("", "atmos-templates-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	// Ensure directory permissions are restricted.
	if err := os.Chmod(tempDir, defaultDirPermissions); err != nil {
		return "", fmt.Errorf("failed to set temp directory permissions: %w", err)
	}
	return tempDir, nil
}

// Write merged JSON data to a temporary file and return its final file URL.
func writeMergedDataToFile(tempDir string, mergedData map[string]interface{}) (*url.URL, error) {
	// Write the merged JSON data to a file.
	rawJSON, err := json.Marshal(mergedData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged data to JSON: %w", err)
	}

	// Create a temporary file inside the temp directory.
	tmpfile, err := os.CreateTemp(tempDir, "gomplate-data-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp data file for gomplate: %w", err)
	}
	tmpName := tmpfile.Name()
	if _, err := tmpfile.Write(rawJSON); err != nil {
		tmpfile.Close()
		return nil, fmt.Errorf("failed to write JSON to temp file: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp data file: %w", err)
	}

	fileURL := toFileScheme(tmpName)

	finalFileUrl, err := fixWindowsFileScheme(fileURL)
	if err != nil {
		return nil, err
	}
	return finalFileUrl, nil
}

// Write the 'outer' top-level file and return its final file URL.
func writeOuterTopLevelFile(tempDir string, fileURL string) (*url.URL, error) {
	// Write the 'outer' top-level file.
	topLevel := map[string]interface{}{
		"Env": map[string]interface{}{
			"README_YAML": fileURL,
		},
	}
	outerJSON, err := json.Marshal(topLevel)
	if err != nil {
		return nil, err
	}

	tmpfile2, err := os.CreateTemp(tempDir, "gomplate-top-level-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp data file for top-level: %w", err)
	}
	tmpName2 := tmpfile2.Name()
	if _, err = tmpfile2.Write(outerJSON); err != nil {
		tmpfile2.Close()
		return nil, fmt.Errorf("failed to write top-level JSON: %w", err)
	}
	if err = tmpfile2.Close(); err != nil {
		return nil, fmt.Errorf("failed to close top-level JSON: %w", err)
	}

	topLevelFileURL := toFileScheme(tmpName2)

	finalTopLevelFileURL, err := fixWindowsFileScheme(topLevelFileURL)
	if err != nil {
		return nil, err
	}
	return finalTopLevelFileURL, nil
}

// ProcessTmplWithDatasourcesGomplate parses and executes Go templates with datasources using Gomplate.
func ProcessTmplWithDatasourcesGomplate(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	mergedData map[string]interface{},
	ignoreMissingTemplateValues bool,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ProcessTmplWithDatasourcesGomplate")()

	tempDir, err := createTempDirectory()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	finalFileUrl, err := writeMergedDataToFile(tempDir, mergedData)
	if err != nil {
		return "", err
	}

	finalTopLevelFileURL, err := writeOuterTopLevelFile(tempDir, finalFileUrl.String())
	if err != nil {
		return "", err
	}

	// Construct Gomplate Options.
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

	// Render the template.
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
