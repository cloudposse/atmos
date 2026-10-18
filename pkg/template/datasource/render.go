// Package datasource renders Gomplate templates with merged YAML/JSON metadata.
package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"text/template"

	"github.com/hairyhenderson/gomplate/v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
		Funcs: funcs,
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
