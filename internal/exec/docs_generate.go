package exec

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"gopkg.in/yaml.v3"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"

	tfdocsMarkdown "github.com/terraform-docs/terraform-docs/format"
	tfdocsPrint "github.com/terraform-docs/terraform-docs/print"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"
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

	// Validate basic config
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
	// Merge data from all YAML inputs
	mergedData := map[string]interface{}{}

	for _, src := range docsGenerate.Input {
		dataMap, err := fetchAndParseYAML(src, dir)
		if err != nil {
			u.LogTrace(atmosConfig, fmt.Sprintf("Skipping input '%s' due to error: %v", src, err))
			continue
		}
		mergedData = deepMerge(mergedData, dataMap)
	}

	localReadmeYaml := filepath.Join(dir, "README.yaml")
	if fileExists(localReadmeYaml) {
		localData, err := parseYAML(localReadmeYaml)
		if err == nil {
			mergedData = deepMerge(mergedData, localData)
		} else {
			u.LogDebug(atmosConfig, fmt.Sprintf("Error reading local README.yaml: %v", err))
		}
	}

	if docsGenerate.Terraform.Enabled {
		terraformDocs, err := runTerraformDocs(dir, docsGenerate.Terraform)
		if err != nil {
			return fmt.Errorf("failed to generate terraform docs: %w", err)
		}
		mergedData["terraform_docs"] = terraformDocs
	}

	chosenTemplate, err := pickFirstAvailableTemplate(docsGenerate.Template, dir)
	if err != nil {
		// fallback
		chosenTemplate = "# {{ .name | default \"Project\" }}\n\n{{ .description | default \"No description.\"}}\n\n{{ .terraform_docs }}"
	}

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

// fetchAndParseYAML loads a remote or local path into a map[string]interface{}
func fetchAndParseYAML(pathOrURL string, baseDir string) (map[string]interface{}, error) {
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		resp, err := http.Get(pathOrURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP error %d fetching %s", resp.StatusCode, pathOrURL)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return parseYAMLBytes(body)
	}

	candidate := pathOrURL
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	if !fileExists(candidate) {
		return nil, fmt.Errorf("file does not exist: %s", candidate)
	}
	return parseYAML(candidate)
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

// runTerraformDocs calls terraform-docs as a library at v0.19.0
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

	doc := formatter.Content()

	return doc, nil
}

// pickFirstAvailableTemplate tries each template in order, returning the first that can be found/read.
func pickFirstAvailableTemplate(templates []string, baseDir string) (string, error) {
	for _, t := range templates {
		content, err := fetchTemplate(t, baseDir)
		if err == nil && content != "" {
			return content, nil
		}
	}
	return "", errors.New("no valid template found in the list")
}

func fetchTemplate(pathOrURL string, baseDir string) (string, error) {
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		resp, err := http.Get(pathOrURL)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP error %d for %s", resp.StatusCode, pathOrURL)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	candidate := pathOrURL
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	if !fileExists(candidate) {
		return "", fmt.Errorf("file not found: %s", candidate)
	}
	b, err := os.ReadFile(candidate)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func inMemoryYaml(m map[string]interface{}) string {
	b, _ := yaml.Marshal(m)
	return "string://" + string(b)
}

// TBC: Check if it can be replaced with mergo as it is done in other places
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if existing, ok := out[k]; ok {
			existingMap, eOk := existing.(map[string]interface{})
			incomingMap, iOk := v.(map[string]interface{})
			if eOk && iOk {
				out[k] = deepMerge(existingMap, incomingMap)
			} else {
				out[k] = v
			}
		} else {
			out[k] = v
		}
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
