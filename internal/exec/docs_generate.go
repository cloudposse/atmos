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
	u "github.com/cloudposse/atmos/pkg/utils"

	tfdocsMarkdown "github.com/terraform-docs/terraform-docs/format"
	tfdocsPrint "github.com/terraform-docs/terraform-docs/print"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"

	"github.com/hashicorp/go-getter"
)

// ExecuteDocsGenerateCmd implements the 'atmos docs generate' logic
func ExecuteDocsGenerateCmd(cmd *cobra.Command, args []string) error {
	// 1) Parse CLI flags/args
	flags := cmd.Flags()
	all, err := flags.GetBool("all")
	if err != nil {
		return err
	}

	var targetDir string
	if len(args) > 0 {
		targetDir = args[0]
	} else {
		// default to current directory
		targetDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	docsGenerate := atmosConfig.Settings.Docs.Generate

	if len(docsGenerate.Input) == 0 {
		u.LogDebug(atmosConfig, "No 'docs.generate.input' sources defined in atmos.yaml.")
	}
	if len(docsGenerate.Template) == 0 {
		u.LogDebug(atmosConfig, "No 'docs.generate.template' is defined, generating minimal readme.")
	}

	// If `--all`, walk subdirectories. Otherwise, process just one.
	if all {
		return generateAllReadmes(atmosConfig, targetDir, docsGenerate)
	} else {
		return generateSingleReadme(atmosConfig, targetDir, docsGenerate)
	}
}

// generateAllReadmes searches recursively for any README.yaml, merges data, and writes README.md
func generateAllReadmes(atmosConfig schema.AtmosConfiguration, baseDir string, docsGenerate schema.DocsGenerate) error {
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		// If you eventually reintroduce local reading, you'd look for "README.yaml" or similar here.
		if strings.EqualFold(info.Name(), "README.yaml") {
			dir := filepath.Dir(path)
			err := generateSingleReadme(atmosConfig, dir, docsGenerate)
			if err != nil {
				return fmt.Errorf("error generating docs in %s: %w", dir, err)
			}
		}
		return nil
	})
	return err
}

func generateSingleReadme(atmosConfig schema.AtmosConfiguration, dir string, docsGenerate schema.DocsGenerate) error {
	// 1) Merge data from all YAML inputs
	var allMaps []map[string]any
	for _, src := range docsGenerate.Input {
		dataMap, err := fetchAndParseYAML(atmosConfig, src, dir)
		if err != nil {
			u.LogTrace(atmosConfig, fmt.Sprintf("Skipping input '%s' due to error: %v", src, err))
			continue
		}
		allMaps = append(allMaps, dataMap)
	}
	// Use the native Atmos merging:
	mergedData, err := merge.Merge(atmosConfig, allMaps)
	if err != nil {
		return fmt.Errorf("failed to merge input YAMLs: %w", err)
	}
	// 2) Generate terraform docs if configured
	if docsGenerate.Terraform.Enabled {
		terraformDocs, err := runTerraformDocs(dir, docsGenerate.Terraform)
		if err != nil {
			return fmt.Errorf("failed to generate terraform docs: %w", err)
		}
		mergedData["terraform_docs"] = terraformDocs
	}

	// 3) Fetch the template (via go-getter) or fallback
	var chosenTemplate string
	if docsGenerate.Template != "" {
		templateFile, tempDir, err := downloadSource(atmosConfig, docsGenerate.Template, dir)
		if err != nil {
			u.LogDebug(atmosConfig, fmt.Sprintf("Error fetching template '%s': %v. Using fallback instead.", docsGenerate.Template, err))
		} else {
			defer removeTempDir(atmosConfig, tempDir)
			body, err := os.ReadFile(templateFile)
			if err == nil {
				chosenTemplate = string(body)
			} else {
				u.LogDebug(atmosConfig, fmt.Sprintf("Error reading template file: %v. Using fallback.", err))
			}
		}
	}

	// 4) If no user-supplied template or if an error occurred, fallback
	if chosenTemplate == "" {
		chosenTemplate = "# {{ .name | default \"Project\" }}\n\n{{ .description | default \"No description.\"}}\n\n{{ .terraform_docs }}"
	}

	// 5) Render final README with existing gomplate call
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

	// 6) Write final README.md (or custom output name)
	outputFile := docsGenerate.Output
	if outputFile == "" {
		outputFile = "README.md"
	}
	outputPath := filepath.Join(dir, outputFile)

	if err = os.WriteFile(outputPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("failed to write output %s: %w", outputPath, err)
	}

	u.LogInfo(atmosConfig, fmt.Sprintf("Generated docs at %s", outputPath))
	return nil
}

// fetchAndParseYAML fetches a YAML file from a local path or URL, parses it, and returns the data
func fetchAndParseYAML(atmosConfig schema.AtmosConfiguration, pathOrURL string, baseDir string) (map[string]interface{}, error) {
	localPath, temDir, err := downloadSource(atmosConfig, pathOrURL, baseDir)
	defer removeTempDir(atmosConfig, temDir)
	if err != nil {
		return nil, err
	}
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

func runTerraformDocs(dir string, settings schema.TerraformDocsSettings) (string, error) {
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

	// If path is relative & not obviously remote, make it absolute
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

// findSingleFileInDir enforces that exactly one file is present
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
