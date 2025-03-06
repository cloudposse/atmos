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

// ExecuteDocsGenerateCmd implements the 'atmos docs generate' logic.
//
// This command:
// 1. Processes command-line arguments once at the start.
// 2. Uses os.Chdir to move into subdirectories that have a local 'atmos.yaml'.
// 3. Calls cfg.InitCliConfig(infoLocal, false) in each subdirectory to load its config, then returns (chdir) to the original directory.
// 4. If no local 'atmos.yaml' is found, falls back to the root config.
func ExecuteDocsGenerateCmd(cmd *cobra.Command, args []string) error {
	// 1) Parse CLI flags/args
	flags := cmd.Flags()
	all, err := flags.GetBool("all")
	if err != nil {
		return err
	}

	var targetDir string
	currDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		targetDir = filepath.Join(currDir, args[0])
	} else {
		// default to current directory
		targetDir = currDir
	}

	// Load our root config once using the current working directory
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}
	rootConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	docsGenerate := rootConfig.Settings.Docs.Generate
	if len(docsGenerate.Input) == 0 {
		log.Debug("No 'docs.generate.input' sources defined in atmos.yaml.")
	}
	if len(docsGenerate.Template) == 0 {
		log.Debug("No 'docs.generate.template' is defined, generating minimal readme.")
	}

	// If `--all`, walk subdirectories. Otherwise, process just one.
	if all {
		return generateAllReadmesWithChdir(rootConfig, info, targetDir)
	} else {
		return generateSingleReadmeWithChdir(rootConfig, info, targetDir, docsGenerate)
	}
}

// generateAllReadmesWithChdir scans for README.yaml in subdirectories:
//
// - If the subdir has a local 'atmos.yaml':
//  1. Save oldDir = os.Getwd()
//  2. os.Chdir(subDir)
//  3. Create a copy of the original info => infoLocal
//  4. Call cfg.InitCliConfig(infoLocal, false) to load that subdirectory's config
//  5. Chdir back to oldDir
//  6. Generate docs with the newly loaded config
//
// - If no local 'atmos.yaml' exists, just use the root config as a fallback.
func generateAllReadmesWithChdir(
	rootConfig schema.AtmosConfiguration,
	rootInfo schema.ConfigAndStacksInfo,
	baseDir string,
) error {
	err := filepath.Walk(baseDir, func(path string, infoFile os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if strings.EqualFold(infoFile.Name(), "README.yaml") {
			subDir := filepath.Dir(path)

			// Check if there's a local atmos.yaml in subDir
			localAtmos := filepath.Join(subDir, "atmos.yaml")

			// If found, we do an os.Chdir + re-init config from that location
			if fileExists(localAtmos) {
				oldDir, err := os.Getwd()
				if err != nil {
					return err
				}
				defer os.Chdir(oldDir) // ensure we go back no matter what

				if err := os.Chdir(subDir); err != nil {
					return fmt.Errorf("failed to chdir into %s: %w", subDir, err)
				}

				// reuse the same info, just copy it
				infoLocal := rootInfo
				// we do not re-parse flags, we trust them to be the same
				localConfig, localErr := cfg.InitCliConfig(infoLocal, false)
				if localErr != nil {
					// fallback to root config
					log.Debug("Error loading local atmos.yaml", "directory", subDir, "error", localErr)
					return generateSingleReadme(rootConfig, subDir, rootConfig.Settings.Docs.Generate)
				}

				// Now we have a subdir config; let's use it
				return generateSingleReadme(localConfig, subDir, localConfig.Settings.Docs.Generate)
			}

			// If no local atmos.yaml found, fallback to root config
			return generateSingleReadme(rootConfig, subDir, rootConfig.Settings.Docs.Generate)
		}
		return nil
	})
	return err
}

// generateSingleReadmeWithChdir handles the scenario of 'atmos docs generate [path]' for a single directory,
// ignoring the --all flag.
//
// - If a local 'atmos.yaml' exists in the specified path, os.Chdir is used and config is reinitialized.
// - Otherwise, falls back to the root config from the main call.
func generateSingleReadmeWithChdir(
	rootConfig schema.AtmosConfiguration,
	rootInfo schema.ConfigAndStacksInfo,
	targetDir string,
	rootDocsGenerate schema.DocsGenerate,
) error {
	// Check if subDir has a local atmos.yaml
	localAtmos := filepath.Join(targetDir, "atmos.yaml")
	if fileExists(localAtmos) {
		oldDir, err := os.Getwd()
		if err != nil {
			return err
		}
		defer os.Chdir(oldDir)

		if err := os.Chdir(targetDir); err != nil {
			return fmt.Errorf("failed to chdir into %s: %w", targetDir, err)
		}

		infoLocal := rootInfo
		localConfig, localErr := cfg.InitCliConfig(infoLocal, false)
		if localErr == nil {
			// If we got a local config, use it
			return generateSingleReadme(localConfig, targetDir, localConfig.Settings.Docs.Generate)
		}
		log.Debug("Error loading local atmos.yaml", "directory", targetDir, "error", localErr)
	}

	// fallback to root config if no local atmos or we failed to load it
	return generateSingleReadme(rootConfig, targetDir, rootDocsGenerate)
}

// generateSingleReadme merges data from docsGenerate.Input, runs terraform-docs if needed,
// and writes out a final README.
func generateSingleReadme(
	atmosConfig schema.AtmosConfiguration,
	dir string,
	docsGenerate schema.DocsGenerate,
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

	// 2) Generate terraform docs if configured
	if docsGenerate.Terraform.Enabled {
		terraformDocs, err := runTerraformDocs(dir, docsGenerate.Terraform)
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
