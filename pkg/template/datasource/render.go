// Package datasource renders Gomplate templates with merged YAML/JSON metadata.
package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/render"
)

const defaultDirPermissions = 0o700

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
func writeOuterTopLevelFile(tempDir string, fileURL string, mergedData map[string]interface{}) (*url.URL, error) {
	// Keep merged data at the template root for templates that use `.name`,
	// while preserving the historical Env.README_YAML helper.
	topLevel := make(map[string]interface{}, len(mergedData))
	for k, v := range mergedData {
		topLevel[k] = v
	}

	envData := map[string]interface{}{}
	if existingEnv, ok := topLevel["Env"].(map[string]interface{}); ok {
		for k, v := range existingEnv {
			envData[k] = v
		}
	}
	envData["README_YAML"] = fileURL
	topLevel["Env"] = envData

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

// Options customizes one render.
type Options struct {
	// Funcs are scoped to this render; their return values retain native
	// fields and methods, bypassing datasource JSON serialization.
	Funcs template.FuncMap
	// WrapFuncs receives the fully assembled function map and the render's
	// datasources, which include "." and "config". Policy enforcement uses it
	// to guard every function call.
	WrapFuncs render.FuncsWrapper
	// MissingKey is the text/template "missingkey" option; empty means error.
	MissingKey string
}

// Render applies a Gomplate template with config and root datasources.
// Custom functions are scoped to this render and their return values retain
// native fields and methods, bypassing datasource JSON serialization.
func Render(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	mergedData map[string]interface{},
	funcs template.FuncMap,
) (string, error) {
	defer perf.Track(atmosConfig, "datasource.Render")()

	return RenderWith(atmosConfig, tmplName, tmplValue, mergedData, &Options{Funcs: funcs})
}

// RenderWith is Render with every option. The merged data is exposed on `.`
// (with the historical `.Env.README_YAML` helper) and as the `config`
// datasource; only Gomplate functions and opts.Funcs are available.
func RenderWith(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	mergedData map[string]interface{},
	opts *Options,
) (string, error) {
	defer perf.Track(atmosConfig, "datasource.RenderWith")()

	if opts == nil {
		opts = &Options{}
	}

	tempDir, err := createTempDirectory()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	finalFileUrl, err := writeMergedDataToFile(tempDir, mergedData)
	if err != nil {
		return "", err
	}

	finalTopLevelFileURL, err := writeOuterTopLevelFile(tempDir, finalFileUrl.String(), mergedData)
	if err != nil {
		return "", err
	}

	engine := render.New(render.WithSprig(false))
	return engine.Render(context.Background(), &render.Request{
		Name:       tmplName,
		Text:       tmplValue,
		Funcs:      opts.Funcs,
		WrapFuncs:  opts.WrapFuncs,
		MissingKey: opts.MissingKey,
		ContextSources: map[string]render.Datasource{
			".":      {URL: finalTopLevelFileURL.String()},
			"config": {URL: finalFileUrl.String()},
		},
	})
}
