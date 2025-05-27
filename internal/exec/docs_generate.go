package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"

	tfdocsFormat "github.com/terraform-docs/terraform-docs/format"
	tfdocsPrint "github.com/terraform-docs/terraform-docs/print"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"

	log "github.com/charmbracelet/log"
)

const (
	defaultReadmeOutput    = "./README.md"
	defaultDirPermissions  = 0o700
	defaultFilePermissions = 0o644
)

// TemplateRenderer is an interface for rendering templates.
type TemplateRenderer interface {
	Render(tmplName, tmplValue string, mergedData map[string]interface{}, ignoreMissing bool) (string, error)
}

// defaultTemplateRenderer is the production implementation of TemplateRenderer.
type defaultTemplateRenderer struct{}

// Render delegates rendering to the existing ProcessTmplWithDatasourcesGomplate function.
func (d defaultTemplateRenderer) Render(tmplName, tmplValue string, mergedData map[string]interface{}, ignoreMissing bool) (string, error) {
	return ProcessTmplWithDatasourcesGomplate(tmplName, tmplValue, mergedData, ignoreMissing)
}

// ExecuteDocsGenerateCmd implements the 'atmos docs generate <doc-type>' logic.
func ExecuteDocsGenerateCmd(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return ErrMissingDocType
	}
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}
	rootConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	// Retrieve the <doc-type> generation config from atmos.yaml.
	// The target directory is taken from docs.generate.<doc-type>.base-dir.
	docsGenerate, ok := rootConfig.Docs.Generate[args[0]]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoDocsGenerateEntry, args[0])
	}

	basedir := docsGenerate.BaseDir
	var targetDir string
	currDir, err := os.Getwd()
	if err != nil {
		return err
	}
	log.Debug("Current directory", "dir", currDir, "basedir", basedir)

	targetDir = filepath.Join(currDir, basedir)

	if len(docsGenerate.Input) == 0 {
		log.Debug("docs.generate['<doc-type>'].input is empty in atmos.yaml")
	}
	if len(docsGenerate.Template) == 0 {
		log.Debug("No docs.generate['<doc-type>'].template' defined, generating minimal <doc-type>.")
	}

	// Generate a single document in the targetDir using the default renderer.
	return generateDocument(&rootConfig, targetDir, &docsGenerate, defaultTemplateRenderer{})
}

// mergeInputs merges all YAML inputs defined in docsGenerate.Input.
func mergeInputs(cfg *schema.AtmosConfiguration, dir string, dg *schema.DocsGenerate) (map[string]any, error) {
	var allMaps []map[string]any
	for _, src := range dg.Input {
		var dataMap map[string]any
		switch v := src.(type) {
		case string:
			// fetch from file or URL
			m, err := fetchAndParseYAML(cfg, v, dir)
			if err != nil {
				log.Debug("skipping input", "input", v, "err", err)
				continue
			}
			dataMap = m

		case map[string]any:
			// inline map
			dataMap = v

		default:
			return nil, fmt.Errorf("%w: %T", ErrUnsupportedInputType, v)
		}
		allMaps = append(allMaps, dataMap)
	}
	if len(allMaps) == 0 {
		return map[string]any{}, nil
	}
	return merge.Merge(*cfg, allMaps)
}

// getTerraformSource returns the directory to use for generating Terraform docs.
// If a source is specified, it verifies that the joined path exists and is a directory.
// Otherwise, it returns the base directory.
func getTerraformSource(dir, source string) (string, error) {
	if source != "" {
		joinedPath := filepath.Join(dir, source)
		stat, err := os.Stat(joinedPath)
		if err != nil || !stat.IsDir() {
			return "", fmt.Errorf("%w: %s", ErrSourceDirNotExist, joinedPath)
		}
		return joinedPath, nil
	}
	return dir, nil
}

// getTemplateContent downloads and reads the template file from the given URL.
func getTemplateContent(atmosConfig *schema.AtmosConfiguration, templateURL, dir string) (string, error) {
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

func applyTerraformDocs(dir string, docsGenerate *schema.DocsGenerate, mergedData map[string]any) error {
	if !docsGenerate.Terraform.Enabled {
		return nil
	}

	terraformSource, err := getTerraformSource(dir, docsGenerate.Terraform.Source)
	if err != nil {
		log.Debug("Skipping terraform docs generation", "error", err)
		return nil
	}

	terraformDocs, err := runTerraformDocs(terraformSource, &docsGenerate.Terraform)
	if err != nil {
		return fmt.Errorf("failed to generate terraform docs: %w", err)
	}
	mergedData["terraform_docs"] = terraformDocs
	return nil
}

func fetchTemplate(atmosConfig *schema.AtmosConfiguration, docsGenerate *schema.DocsGenerate, dir string) string {
	if docsGenerate.Template != "" {
		tmpl, err := getTemplateContent(atmosConfig, docsGenerate.Template, dir)
		if err == nil {
			return tmpl
		}
		log.Debug("Error fetching template", "template", docsGenerate.Template, "error", err)
	}
	// Return default template if none is provided or on error.
	return "# {{ .name | default \"Project\" }}\n\n{{ .description | default \"No description.\"}}\n\n{{ .terraform_docs }}"
}

// generateDocument merges docs inputs, optionally runs terraform-docs, renders, and writes the document.
func generateDocument(
	atmosConfig *schema.AtmosConfiguration,
	baseDir string,
	docsGenerate *schema.DocsGenerate,
	renderer TemplateRenderer,
) error {
	// 1) Merge YAML inputs.
	mergedData, err := mergeInputs(atmosConfig, baseDir, docsGenerate)
	if err != nil {
		return fmt.Errorf("failed to merge input YAMLs: %w", err)
	}

	// 2) Generate terraform docs if enabled.
	if err := applyTerraformDocs(baseDir, docsGenerate, mergedData); err != nil {
		return err
	}

	// 3) Fetch the template.
	chosenTemplate := fetchTemplate(atmosConfig, docsGenerate, baseDir)

	// 4) Render final document using the injected renderer.
	rendered, err := renderer.Render("docs-generate", chosenTemplate, mergedData, true)
	if err != nil {
		return fmt.Errorf("failed to render template with datasources: %w", err)
	}

	// 5) Resolve and write final document.
	outputFile := docsGenerate.Output
	if outputFile == "" {
		outputFile = defaultReadmeOutput
	}
	outputPath, err := resolvePath(outputFile, baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output path %s: %w", outputFile, err)
	}
	if err = os.WriteFile(outputPath, []byte(rendered), defaultFilePermissions); err != nil {
		return fmt.Errorf("failed to write output %s: %w", outputPath, err)
	}

	log.Info("Generated docs", "output", outputPath)
	return nil
}

// fetchAndParseYAML fetches a YAML file from a local path or URL, parses it, and returns the data.
func fetchAndParseYAML(atmosConfig *schema.AtmosConfiguration, pathOrURL string, baseDir string) (map[string]interface{}, error) {
	localPath, tempDir, err := downloadSource(atmosConfig, pathOrURL, baseDir)
	if err != nil {
		return nil, err
	}
	defer removeTempDir(tempDir)
	return parseYAML(localPath)
}

func parseYAML(filePath string) (map[string]interface{}, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return parseYAMLBytes(content)
}

func parseYAMLBytes(b []byte) (map[string]interface{}, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, nil
}

type Formatter interface {
	Generate(module *tfdocsTf.Module) error
	Content() string
}

func runTerraformDocs(dir string, settings *schema.TerraformDocsReadmeSettings) (string, error) {
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
		return "", fmt.Errorf("failed to load terraform module: %w", err)
	}

	var formatter Formatter

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
		return "", err
	}
	return formatter.Content(), nil
}

// downloadSource calls the go-getter and returns file path.
func downloadSource(
	atmosConfig *schema.AtmosConfiguration,
	pathOrURL string,
	baseDir string,
) (localPath string, temDir string, err error) {
	// If path is not remote, resolve it.
	if !isRemoteSource(pathOrURL) {
		pathOrURL, err = resolvePath(pathOrURL, baseDir)
		if err != nil {
			return "", "", fmt.Errorf("%w: %s", ErrPathResolution, err)
		}
	}
	log.Debug("Downloading source", "source", pathOrURL, "baseDir", baseDir)
	tempDir, err := os.MkdirTemp("", "atmos-docs-*")
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrCreateTempDir, err)
	}
	// Ensure directory permissions are restricted.
	if err := os.Chmod(tempDir, defaultDirPermissions); err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrSetTempDirPermissions, err)
	}

	log.Debug("Downloading source", "source", pathOrURL, "tempDir", tempDir)

	if err := downloader.NewGoGetterDownloader(atmosConfig).Fetch(pathOrURL, tempDir, downloader.ClientModeAny, 10*time.Minute); err != nil {
		return "", tempDir, fmt.Errorf("%w: %s: %v", ErrDownloadPackage, pathOrURL, err)
	}

	fileName := filepath.Base(pathOrURL)
	downloadedPath := filepath.Join(tempDir, fileName)

	return downloadedPath, tempDir, nil
}

// isLikelyRemote does a quick check if a path looks remote.
func isRemoteSource(s string) bool {
	prefixes := []string{"http://", "https://", "git::", "github.com/", "git@"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// - Explicit relative (./ or ../) relative to cwd.
// - Implicit relative paths first against baseDir, then cwd.
func resolvePath(path string, baseDir string) (string, error) {
	if path == "" {
		return "", ErrEmptyFilePath
	}

	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "../") {
		// Implicit relative path: resolve first against baseDir
		path = filepath.Join(baseDir, path)
	}

	// Finally, resolve against cwd
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrPathResolution, err)
	}

	return absPath, nil
}
