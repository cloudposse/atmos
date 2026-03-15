package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
	"github.com/cloudposse/atmos/pkg/lint/rules"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteLintStacksCmd executes `atmos lint stacks`.
func ExecuteLintStacksCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteLintStacksCmd")()

	s := spinner.New("Linting Atmos Stacks...")
	s.Start()

	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		s.Stop()
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		s.Stop()
		return err
	}

	flags := cmd.Flags()

	stackFilter, _ := flags.GetString("stack")
	ruleFlag, _ := flags.GetString("rule")
	formatFlag, _ := flags.GetString("format")
	severityFlag, _ := flags.GetString("severity")

	var ruleIDs []string
	if ruleFlag != "" {
		for _, r := range strings.Split(ruleFlag, ",") {
			ruleIDs = append(ruleIDs, strings.TrimSpace(r))
		}
	}

	minSeverity := lint.SeverityInfo
	switch strings.ToLower(severityFlag) {
	case "warning", "warn":
		minSeverity = lint.SeverityWarning
	case "error":
		minSeverity = lint.SeverityError
	}

	result, err := LintStacks(&atmosConfig, stackFilter, ruleIDs, minSeverity)
	if err != nil {
		s.Error("Stack linting failed")
		return err
	}

	s.Stop()

	// Render output.
	switch strings.ToLower(formatFlag) {
	case "json":
		return renderLintJSON(result)
	default:
		renderLintText(result, &atmosConfig)
	}

	if result.HasErrors() {
		return fmt.Errorf("lint stacks found %d error(s)", result.Summary.Errors)
	}

	return nil
}

// LintStacks runs the lint engine against all loaded stacks.
func LintStacks(
	atmosConfig *schema.AtmosConfiguration,
	stackFilter string,
	ruleIDs []string,
	minSeverity lint.Severity,
) (*lint.LintResult, error) {
	defer perf.Track(atmosConfig, "exec.LintStacks")()

	// Load all stacks.
	stacksMap, rawStackConfigs, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return nil, err
	}

	// Filter to a specific stack if requested.
	if stackFilter != "" {
		filtered := make(map[string]any)
		for name, v := range stacksMap {
			if name == stackFilter || strings.Contains(name, stackFilter) {
				filtered[name] = v
			}
		}
		stacksMap = filtered
	}

	// Build import graph from raw stack configs.
	importGraph := buildImportGraph(rawStackConfigs)

	// Find all YAML files under the stacks base path for orphan detection (L-07).
	// Use the existing utility and convert relative paths back to absolute,
	// excluding template files since they are not directly referenced by imports.
	allStackFiles, err := stackYAMLFiles(atmosConfig.StacksBaseAbsolutePath)
	if err != nil {
		log.Debug("Could not enumerate stack files for orphan detection", "error", err)
		allStackFiles = nil
	}

	// Merge lint config defaults.
	lintConfig := mergedLintConfig(atmosConfig.Lint)

	ctx := lint.LintContext{
		StacksMap:       stacksMap,
		RawStackConfigs: rawStackConfigs,
		ImportGraph:     importGraph,
		StacksBasePath:  atmosConfig.StacksBaseAbsolutePath,
		AllStackFiles:   allStackFiles,
		AtmosConfig:     *atmosConfig,
		LintConfig:      lintConfig,
	}

	engine := lint.NewEngine(rules.All())
	return engine.Run(ctx, ruleIDs, minSeverity)
}

// buildImportGraph constructs a file → []importedFiles map from raw stack configs.
func buildImportGraph(rawStackConfigs map[string]map[string]any) map[string][]string {
	graph := make(map[string][]string)
	for filePath, rawConfig := range rawStackConfigs {
		rawImports, ok := rawConfig[cfg.ImportSectionName]
		if !ok {
			continue
		}
		var imports []string
		switch v := rawImports.(type) {
		case []string:
			imports = v
		case []any:
			for _, item := range v {
				switch s := item.(type) {
				case string:
					imports = append(imports, s)
				case map[string]any:
					if path, ok := s["path"].(string); ok {
						imports = append(imports, path)
					}
				}
			}
		}
		if len(imports) > 0 {
			graph[filePath] = imports
		}
	}
	return graph
}

// stackYAMLFiles returns all non-template YAML files under root as absolute paths,
// delegating to u.GetAllYamlFilesInDir and filtering out .tmpl files.
func stackYAMLFiles(root string) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	relFiles, err := u.GetAllYamlFilesInDir(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(relFiles))
	for _, rel := range relFiles {
		// Skip template files — they are not referenced by import chains.
		if strings.HasSuffix(rel, ".tmpl") {
			continue
		}
		files = append(files, filepath.Join(root, rel))
	}
	return files, nil
}

// mergedLintConfig returns a LintConfig with defaults applied for missing fields.
func mergedLintConfig(cfg schema.LintConfig) schema.LintConfig {
	if cfg.MaxImportDepth <= 0 {
		cfg.MaxImportDepth = 3
	}
	if cfg.DRYThresholdPct <= 0 {
		cfg.DRYThresholdPct = 80
	}
	if len(cfg.SensitiveVarPatterns) == 0 {
		cfg.SensitiveVarPatterns = []string{
			"*password*", "*secret*", "*token*", "*key*",
			"*arn*", "*account_id*", "*role*",
		}
	}
	if cfg.Rules == nil {
		cfg.Rules = map[string]string{
			"L-01": "warning",
			"L-02": "warning",
			"L-03": "warning",
			"L-04": "error",
			"L-05": "info",
			"L-06": "info",
			"L-07": "warning",
			"L-08": "warning",
			"L-09": "error",
			"L-10": "warning",
		}
	}
	return cfg
}

// renderLintText renders findings in human-readable text format.
func renderLintText(result *lint.LintResult, atmosConfig *schema.AtmosConfiguration) {
	if len(result.Findings) == 0 {
		u.PrintMessage("✓ No lint findings.")
		return
	}

	// Group by severity.
	groups := map[lint.Severity][]lint.LintFinding{
		lint.SeverityError:   {},
		lint.SeverityWarning: {},
		lint.SeverityInfo:    {},
	}
	for _, f := range result.Findings {
		groups[f.Severity] = append(groups[f.Severity], f)
	}

	for _, sev := range []lint.Severity{lint.SeverityError, lint.SeverityWarning, lint.SeverityInfo} {
		findings := groups[sev]
		if len(findings) == 0 {
			continue
		}

		// Sort by file then rule ID.
		sort.Slice(findings, func(i, j int) bool {
			if findings[i].File != findings[j].File {
				return findings[i].File < findings[j].File
			}
			return findings[i].RuleID < findings[j].RuleID
		})

		for _, f := range findings {
			icon := severityIcon(f.Severity)
			location := f.File
			if f.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, f.Line)
			}
			if location != "" {
				u.PrintMessage(fmt.Sprintf("%s [%s] %s  (%s)", icon, f.RuleID, f.Message, location))
			} else {
				u.PrintMessage(fmt.Sprintf("%s [%s] %s", icon, f.RuleID, f.Message))
			}
			if f.FixHint != "" {
				u.PrintMessage(fmt.Sprintf("  → %s", f.FixHint))
			}
		}
	}

	u.PrintMessage("")
	u.PrintMessage(fmt.Sprintf(
		"Summary: %d error(s), %d warning(s), %d info",
		result.Summary.Errors,
		result.Summary.Warnings,
		result.Summary.Info,
	))
}

// renderLintJSON renders findings as JSON to stdout.
func renderLintJSON(result *lint.LintResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// severityIcon returns a text icon for the given severity.
func severityIcon(s lint.Severity) string {
	switch s {
	case lint.SeverityError:
		return "✗"
	case lint.SeverityWarning:
		return "⚠"
	default:
		return "ℹ"
	}
}
