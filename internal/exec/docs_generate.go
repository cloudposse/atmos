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
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"

	tfdocsMarkdown "github.com/terraform-docs/terraform-docs/format"
	tfdocsPrint "github.com/terraform-docs/terraform-docs/print"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"

	"github.com/hashicorp/go-getter"

	"github.com/charmbracelet/log"
)

// ExecuteDocsGenerateCmd implements the 'atmos docs generate readme' logic.
func ExecuteDocsGenerateCmd(cmd *cobra.Command, args []string) error {
	// Parse CLI flags/args (we ignore any '--all' flag now)
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}
	rootConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	// Retrieve the readme generation config from atmos.yaml.
	// The target directory is taken from docs.generate.readme.base-dir.
	docsGenerate := rootConfig.Settings.Docs.Generate.Readme
	basedir := docsGenerate.BaseDir

	var targetDir string
	currDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		targetDir = filepath.Join(currDir, basedir)
	} else {
		// default to current directory
		targetDir = currDir
	}

	if len(docsGenerate.Input) == 0 {
		log.Debug("No 'docs.generate.readme.input' sources defined in atmos.yaml.")
	}
	if len(docsGenerate.Template) == 0 {
		log.Debug("No 'docs.generate.readme.template' defined, generating minimal readme.")
	}

	// Generate a single README in the targetDir.
	return generateReadme(rootConfig, targetDir, docsGenerate)
}

// generateSingleReadme merges data from docsGenerate.Input, runs terraform-docs if needed,
// and writes out a final README.
func generateReadme(
	atmosConfig schema.AtmosConfiguration,
	dir string,
	docsGenerate schema.DocsGenerateReadme,
) error {
	// 1) Merge data from all YAML inputs
	var allMaps []map[string]any
	for _, src := range docsGenerate.Input {
		dataMap, err := fetchAndParseYAML(atmosConfig, src, dir)
		if err != nil {
			log.Debug("Skipping input due to error", "input", src, "error", err)
			continue
		}
		allMaps = append(allMaps, dataMap)
	}
	// Use the native Atmos merging
	mergedData, err := merge.Merge(atmosConfig, allMaps)
	if err != nil {
		return fmt.Errorf("failed to merge input YAMLs: %w", err)
	}

	// 2) Generate terraform docs if enabled.
	if docsGenerate.Terraform.Enabled {
		// If a source is provided in the configuration, join it with the base directory.
		terraformSource := dir
		if docsGenerate.Terraform.Source != "" {
			terraformSource = filepath.Join(dir, docsGenerate.Terraform.Source)
		}
		terraformDocs, err := runTerraformDocs(terraformSource, docsGenerate.Terraform)
		if err != nil {
			return fmt.Errorf("failed to generate terraform docs: %w", err)
		}
		mergedData["terraform_docs"] = terraformDocs
	}

	// 3) Fetch template (via go-getter) or fallback
	var chosenTemplate string
	if docsGenerate.Template != "" {
		templateFile, tempDir, err := downloadSource(atmosConfig, docsGenerate.Template, dir)
		if err != nil {
			log.Debug("Error fetching template", "template", docsGenerate.Template, "error", err)
		} else {
			defer removeTempDir(atmosConfig, tempDir)
			body, err := os.ReadFile(templateFile)
			if err == nil {
				chosenTemplate = string(body)
			} else {
				log.Debug("Error reading template file", "error", err)
			}
		}
	}
	if chosenTemplate == "" {
		chosenTemplate = "# {{ .name | default \"Project\" }}\n\n{{ .description | default \"No description.\"}}\n\n{{ .terraform_docs }}"
	}

	// 4) Render final README with existing gomplate call
	rendered, err := ProcessTmplWithDatasourcesGomplate(
		atmosConfig,
		schema.Settings{},
		"docs-generate",
		chosenTemplate,
		mergedData,
		true,
	)
	if err != nil {
		return fmt.Errorf("failed to render template with datasources: %w", err)
	}

	// 5) Write final README.md (or custom output name)
	outputFile := docsGenerate.Output
	if outputFile == "" {
		outputFile = "README.md"
	}
	outputPath := filepath.Join(dir, outputFile)

	if err = os.WriteFile(outputPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("failed to write output %s: %w", outputPath, err)
	}

	log.Info("Generated docs", "output", outputPath)
	return nil
}

// fetchAndParseYAML fetches a YAML file from a local path or URL, parses it, and returns the data
func fetchAndParseYAML(atmosConfig schema.AtmosConfiguration, pathOrURL string, baseDir string) (map[string]interface{}, error) {
	localPath, tempDir, err := downloadSource(atmosConfig, pathOrURL, baseDir)
	if err != nil {
		return nil, err
	}
	defer removeTempDir(atmosConfig, tempDir)
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

func runTerraformDocs(dir string, settings schema.TerraformDocsReadmeSettings) (string, error) {
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

	formatter := tfdocsMarkdown.NewMarkdownTable(tfdocsPrint.DefaultConfig())
	if err := formatter.Generate(module); err != nil {
		return "", err
	}

	return formatter.Content(), nil
}

// downloadSource calls the go-getter,
// then returns the single file path if exactly one file is found, or an error otherwise.
func downloadSource(
	atmosConfig schema.AtmosConfiguration,
	pathOrURL string,
	baseDir string,
) (localPath string, temDir string, err error) {
	// If the path is not absolute and not a remote URL, join it with the base directory.
	if !filepath.IsAbs(pathOrURL) && !isLikelyRemote(pathOrURL) {
		pathOrURL = filepath.Join(baseDir, pathOrURL)
	}

	tempDir, err := os.MkdirTemp("", "atmos-docs-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	err = GoGetterGet(atmosConfig, pathOrURL, tempDir, getter.ClientModeAny, 10*time.Minute)
	if err != nil {
		return "", tempDir, fmt.Errorf("failed to download %s: %w", pathOrURL, err)
	}
	downloadedPath, err := findSingleFileInDir(tempDir)
	if err != nil {
		return "", tempDir, fmt.Errorf("go-getter: %w", err)
	}

	return downloadedPath, tempDir, nil
}

/*
go-getter is used to download files as per provided path by user and
the function below is a check that user provided a link to a file or a single file in folder,
not a folder with multiple files, i.e. as of right now we dont have a logic to take just one relevant
template or yml file from the folder downloaded.
So this is why an extra check.
To be decided later: this could be removed
later to introdce simplier logic (like trust users to provide a single file, etc)
*/
func findSingleFileInDir(dir string) (string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no files found in %s", dir)
	}
	if len(files) > 1 {
		return "", fmt.Errorf("multiple files found in %s (found %d)", dir, len(files))
	}
	return files[0], nil
}

// isLikelyRemote does a quick check if a path looks remote
func isLikelyRemote(s string) bool {
	prefixes := []string{"http://", "https://", "git::", "github.com/", "git@"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func fileExists(fp string) bool {
	info, err := os.Stat(fp)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil && !info.IsDir()
}
