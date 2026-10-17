// Package docs generates documents from YAML metadata and native Terraform objects.
package docs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	tfdocsFormat "github.com/terraform-docs/terraform-docs/format"
	tfdocsPrint "github.com/terraform-docs/terraform-docs/print"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/datasource"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	defaultReadmeOutput    = "./README.md"
	defaultDirPermissions  = 0o700
	defaultFilePermissions = 0o644
)

// templateRenderer is an interface for rendering templates.
type templateRenderer interface {
	Render(tmplName, tmplValue string, mergedData map[string]interface{}, module *tfdocsTf.Module) (string, error)
}

// defaultTemplateRenderer is the production implementation of templateRenderer.
type defaultTemplateRenderer struct{}

// Render keeps the native module outside the JSON-backed config datasource so
// templates retain access to methods and fields omitted by JSON serialization.
func (d defaultTemplateRenderer) Render(tmplName, tmplValue string, mergedData map[string]interface{}, module *tfdocsTf.Module) (string, error) {
	return datasource.Render(&schema.AtmosConfiguration{}, tmplName, tmplValue, mergedData, template.FuncMap{
		"terraform": func() *tfdocsTf.Module { return module },
	})
}

// mergeInputs merges all YAML inputs defined in docsGenerate.Input.
func mergeInputs(cfg *schema.AtmosConfiguration, dir string, dg *schema.DocsGenerate) (map[string]any, error) {
	var allMaps []map[string]any
	for _, src := range dg.Input {
		var dataMap map[string]any
		switch v := src.(type) {
		case string:
			m, err := fetchAndParseYAML(cfg, v, dir)
			if err != nil {
				return nil, fmt.Errorf("input %q: %w", v, err)
			}
			dataMap = m

		case map[string]any:
			// inline map
			dataMap = v

		default:
			return nil, fmt.Errorf("%w: %T", errUtils.ErrUnsupportedInputType, v)
		}
		allMaps = append(allMaps, dataMap)
	}
	if len(allMaps) == 0 {
		return map[string]any{}, nil
	}
	return merge.Merge(cfg, allMaps)
}

// getTerraformSource returns the directory to use for generating Terraform docs.
// If a source is specified, it verifies that the joined path exists and is a directory.
// Otherwise, it returns the base directory.
func getTerraformSource(dir, source string) (string, error) {
	if source != "" {
		joinedPath := filepath.Join(dir, source)
		stat, err := os.Stat(joinedPath)
		if err != nil || !stat.IsDir() {
			return "", fmt.Errorf("%w: %s", errUtils.ErrSourceDirNotExist, joinedPath)
		}
		return joinedPath, nil
	}
	return dir, nil
}

// getTemplateContent reads a template from a local file or downloads it from a remote URL.
func getTemplateContent(atmosConfig *schema.AtmosConfiguration, templateURL, dir string) (string, error) {
	if !isRemoteSource(templateURL) {
		templateFile, err := resolvePath(templateURL, dir, "")
		if err != nil {
			return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPathResolution, err)
		}
		body, err := os.ReadFile(templateFile)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	templateFile, tempDir, err := downloadSource(atmosConfig, templateURL, dir)
	if err != nil {
		return "", err
	}
	defer removeTempDir(tempDir)
	body, err := os.ReadFile(templateFile)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// terraformDocsRunner defines an interface for running terraform-docs.
type terraformDocsRunner interface {
	Run(dir string, settings *schema.TerraformDocsReadmeSettings) (terraformDocsResult, error)
}

type terraformDocsResult struct {
	module  *tfdocsTf.Module
	content string
}

// realTerraformDocsRunner is the production implementation.
type realTerraformDocsRunner struct{}

// Run loads a Terraform module and produces its configured documentation format.
func (r realTerraformDocsRunner) Run(dir string, settings *schema.TerraformDocsReadmeSettings) (terraformDocsResult, error) {
	return runTerraformDocs(dir, settings)
}

// applyTerraformDocs adds formatted documentation to metadata and returns the native module.
func applyTerraformDocs(dir string, docsGenerate *schema.DocsGenerate, mergedData map[string]any) (*tfdocsTf.Module, error) {
	return applyTerraformDocsWithRunner(dir, docsGenerate, mergedData, realTerraformDocsRunner{})
}

// applyTerraformDocsWithRunner skips disabled or unavailable sources and propagates generation errors.
func applyTerraformDocsWithRunner(dir string, docsGenerate *schema.DocsGenerate, mergedData map[string]any, runner terraformDocsRunner) (*tfdocsTf.Module, error) {
	if !docsGenerate.Terraform.Enabled {
		return nil, nil
	}

	terraformSource, err := getTerraformSource(dir, docsGenerate.Terraform.Source)
	if err != nil {
		log.Debug("Skipping terraform docs generation", "error", err)
		return nil, nil
	}

	terraformDocs, err := runner.Run(terraformSource, &docsGenerate.Terraform)
	if err != nil {
		return nil, errors.Join(errUtils.ErrGenerateTerraformDocs, err)
	}
	mergedData["terraform_docs"] = terraformDocs.content
	return terraformDocs.module, nil
}

// defaultDocsTemplate combines optional README metadata with formatted Terraform documentation.
func defaultDocsTemplate() string {
	// Read metadata through the "config" datasource, as shared README templates do.
	// Use `index` rather than dot-field access: dot access on a map errors when the key is
	// entirely absent (not just empty), which would defeat the `default` fallback below;
	// `index` returns the zero value instead, letting `default` take over as intended.
	return "# {{ index (ds \"config\") \"name\" | default \"Project\" }}\n\n" +
		"{{ index (ds \"config\") \"description\" | default \"No description.\" }}\n\n" +
		"{{ index (ds \"config\") \"terraform_docs\" | default \"\" }}"
}

// fetchTemplate loads the configured template or returns the built-in default.
func fetchTemplate(atmosConfig *schema.AtmosConfiguration, docsGenerate *schema.DocsGenerate, dir string) (string, error) {
	if docsGenerate.Template == "" {
		return defaultDocsTemplate(), nil
	}

	tmpl, err := getTemplateContent(atmosConfig, docsGenerate.Template, dir)
	if err != nil {
		return "", fmt.Errorf("template %q: %w", docsGenerate.Template, err)
	}
	return tmpl, nil
}

// Generate merges YAML inputs, loads Terraform metadata, and renders the configured document.
func Generate(atmosConfig *schema.AtmosConfiguration, baseDir string, docsGenerate *schema.DocsGenerate) error {
	defer perf.Track(atmosConfig, "docs.Generate")()
	return generateDocument(atmosConfig, baseDir, docsGenerate, defaultTemplateRenderer{})
}

// generateDocument merges the docs inputs, optionally runs terraform-docs, renders, and writes the document.
func generateDocument(
	atmosConfig *schema.AtmosConfiguration,
	baseDir string,
	docsGenerate *schema.DocsGenerate,
	renderer templateRenderer,
) error {
	// 1) Merge YAML inputs.
	mergedData, err := mergeInputs(atmosConfig, baseDir, docsGenerate)
	if err != nil {
		return errors.Join(errUtils.ErrMergeInputYAMLs, err)
	}

	// 2) Generate terraform docs if enabled.
	module, err := applyTerraformDocs(baseDir, docsGenerate, mergedData)
	if err != nil {
		return err
	}
	if _, ok := mergedData["terraform_docs"]; !ok {
		mergedData["terraform_docs"] = ""
	}

	// 3) Fetch the template.
	chosenTemplate, err := fetchTemplate(atmosConfig, docsGenerate, baseDir)
	if err != nil {
		return err
	}

	// 4) Render final document using the injected renderer.
	rendered, err := renderer.Render("docs-generate", chosenTemplate, mergedData, module)
	if err != nil {
		return errors.Join(errUtils.ErrRenderTemplate, err)
	}

	// 5) Resolve and write final document.
	outputPath, err := resolvePath(docsGenerate.Output, baseDir, defaultReadmeOutput)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrResolveOutputPath, docsGenerate.Output, err)
	}
	if err = os.WriteFile(outputPath, []byte(rendered), defaultFilePermissions); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrWriteOutput, outputPath, err)
	}

	ui.Writeln("")
	ui.Success("Generated docs")
	ui.Writeln("")
	log.Debug("Documentation generated", "output", outputPath)
	return nil
}

// fetchAndParseYAML reads a YAML file from a local path or downloads it from a remote URL.
func fetchAndParseYAML(atmosConfig *schema.AtmosConfiguration, pathOrURL string, baseDir string) (map[string]interface{}, error) {
	if !isRemoteSource(pathOrURL) {
		localPath, err := resolvePath(pathOrURL, baseDir, "")
		if err != nil {
			return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPathResolution, err)
		}
		return parseYAML(localPath)
	}

	localPath, tempDir, err := downloadSource(atmosConfig, pathOrURL, baseDir)
	if err != nil {
		return nil, err
	}
	defer removeTempDir(tempDir)
	return parseYAML(localPath)
}

// parseYAML reads a YAML metadata file into a map.
func parseYAML(filePath string) (map[string]interface{}, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return parseYAMLBytes(content)
}

// parseYAMLBytes decodes YAML metadata for merging into the config datasource.
func parseYAMLBytes(b []byte) (map[string]interface{}, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, nil
}

type formatter interface {
	Generate(module *tfdocsTf.Module) error
	Content() string
}

// runTerraformDocs loads the module once and returns both native objects and formatted content.
func runTerraformDocs(dir string, settings *schema.TerraformDocsReadmeSettings) (terraformDocsResult, error) {
	config := tfdocsPrint.DefaultConfig()
	config.ModuleRoot = dir

	config.Sections.Providers = settings.ShowProviders
	config.Sections.Inputs = settings.ShowInputs
	config.Sections.Outputs = settings.ShowOutputs

	if settings.SortBy != "" {
		config.Sort.Enabled = true
		config.Sort.By = settings.SortBy
	}

	module, err := tfdocsTf.LoadWithOptions(config)
	if err != nil {
		return terraformDocsResult{}, fmt.Errorf("failed to load terraform module: %w", err)
	}

	var formatter formatter

	// Assign the correct formatter based on settings.Format.
	switch settings.Format {
	case "markdown table":
		formatter = tfdocsFormat.NewMarkdownTable(tfdocsPrint.DefaultConfig())
	case "markdown":
		formatter = tfdocsFormat.NewMarkdownDocument(tfdocsPrint.DefaultConfig())
	case "tfvars hcl":
		formatter = tfdocsFormat.NewTfvarsHCL(tfdocsPrint.DefaultConfig())
	case "tfvars json":
		formatter = tfdocsFormat.NewTfvarsJSON(tfdocsPrint.DefaultConfig())
	default:
		formatter = tfdocsFormat.NewMarkdownTable(tfdocsPrint.DefaultConfig())
	}

	// Generate content and return it.
	if err := formatter.Generate(module); err != nil {
		return terraformDocsResult{}, err
	}
	return terraformDocsResult{module: module, content: formatter.Content()}, nil
}

// downloadSource calls the go-getter and returns file path.
func downloadSource(
	atmosConfig *schema.AtmosConfiguration,
	pathOrURL string,
	baseDir string,
) (localPath string, temDir string, err error) {
	// If path is not remote, resolve it (no default needed here).
	if !isRemoteSource(pathOrURL) {
		pathOrURL, err = resolvePath(pathOrURL, baseDir, "")
		if err != nil {
			return "", "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPathResolution, err)
		}
	}
	log.Debug("Downloading source", "source", pathOrURL, "baseDir", baseDir)
	tempDir, err := os.MkdirTemp("", "atmos-docs-*")
	if err != nil {
		return "", "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCreateTempDir, err)
	}
	// Ensure directory permissions are restricted.
	if err := os.Chmod(tempDir, defaultDirPermissions); err != nil {
		return "", "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSetTempDirPermissions, err)
	}

	log.Debug("Downloading source", "source", pathOrURL, "tempDir", tempDir)

	if err := downloader.NewGoGetterDownloader(atmosConfig).Fetch(pathOrURL, tempDir, downloader.ClientModeAny, 10*time.Minute); err != nil {
		return "", tempDir, fmt.Errorf("%w: %s: %w", errUtils.ErrDownloadPackage, pathOrURL, err)
	}

	fileName := filepath.Base(pathOrURL)
	downloadedPath := filepath.Join(tempDir, fileName)

	return downloadedPath, tempDir, nil
}

// isRemoteSource checks for URL schemes and go-getter source prefixes.
func isRemoteSource(s string) bool {
	prefixes := []string{"http://", "https://", "git::", "github.com/", "git@"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// resolvePath resolves a file path according to the following rules:
// - If path is empty, uses defaultPath.
// - Absolute paths: resolved as is.
// - Explicit relative (./ or ../) relative to cwd.
// - Implicit relative paths first against baseDir, then cwd.
func resolvePath(path string, baseDir string, defaultPath string) (string, error) {
	if path == "" {
		if defaultPath == "" {
			return "", errUtils.ErrEmptyFilePath
		}
		path = defaultPath
	}

	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "../") {
		// Implicit relative path: resolve first against baseDir
		path = filepath.Join(baseDir, path)
	}

	// Finally, resolve against cwd
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPathResolution, err)
	}

	return absPath, nil
}

// removeTempDir cleans up downloaded inputs, logging cleanup failures.
func removeTempDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		log.Warn(err.Error())
	}
}
