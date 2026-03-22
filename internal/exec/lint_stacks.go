package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
	"github.com/cloudposse/atmos/pkg/lint/rules"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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
		renderLintText(result)
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

	// Filter to a specific stack if requested. Use exact match to be consistent
	// with the rest of Atmos (--stack always means an exact stack name).
	if stackFilter != "" {
		filtered := make(map[string]any)
		for name, v := range stacksMap {
			if name == stackFilter {
				filtered[name] = v
			}
		}
		stacksMap = filtered
	}

	// Build import graph from raw stack configs.
	importGraph := buildImportGraph(rawStackConfigs, atmosConfig.StacksBaseAbsolutePath)

	// Find all YAML files under the stacks base path for orphan detection (L-07).
	// Use the existing utility and convert relative paths back to absolute,
	// excluding template files since they are not directly referenced by imports.
	allStackFiles, err := stackYAMLFiles(atmosConfig.StacksBaseAbsolutePath)
	if err != nil {
		log.Debug("Could not enumerate stack files for orphan detection", "error", err)
		allStackFiles = nil
	}

	// Merge lint config defaults, passing the full atmos config so sensitive_key_patterns
	// from settings.terminal.mask can serve as the single source of truth.
	lintConfig := mergedLintConfig(atmosConfig.Lint.Stacks, atmosConfig.Settings.Terminal.Mask.SensitiveKeyPatterns)

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
// basePath is the absolute stacks base path used to expand glob import patterns.
// Glob patterns (e.g. "catalog/**/*") are expanded to the matching YAML files so
// that L-03 (import depth) and L-07 (orphan detection) produce accurate results.
// If basePath is empty or a glob cannot be expanded, the literal pattern string is
// retained as a best-effort fallback so callers still see partial import data.
func buildImportGraph(rawStackConfigs map[string]map[string]any, basePath string) map[string][]string {
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
			graph[filePath] = expandGlobImports(imports, basePath)
		}
	}
	return graph
}

// expandGlobImports replaces glob import patterns with the matching YAML file paths.
// Non-glob strings are passed through unchanged. If basePath is empty or a glob
// yields no matches, the original pattern is kept so callers retain partial data.
func expandGlobImports(imports []string, basePath string) []string {
	if basePath == "" {
		return imports
	}
	result := make([]string, 0, len(imports))
	for _, imp := range imports {
		if !strings.ContainsAny(imp, "*?[") {
			// Not a glob — pass through unchanged.
			result = append(result, imp)
			continue
		}
		// Try both .yaml and .yml suffixes, and also the raw pattern if it already
		// has an extension.
		patterns := []string{
			filepath.Join(basePath, imp+".yaml"),
			filepath.Join(basePath, imp+".yml"),
			filepath.Join(basePath, imp),
		}
		var matched []string
		for _, pattern := range patterns {
			if hits, err := filepath.Glob(pattern); err == nil {
				matched = append(matched, hits...)
			}
		}
		if len(matched) == 0 {
			// No expansion — keep the literal so callers see the original intent.
			result = append(result, imp)
		} else {
			result = append(result, matched...)
		}
	}
	return result
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

// mergedLintConfig returns a LintStacksConfig with defaults applied for missing fields.
// maskKeyPatterns are glob patterns from settings.terminal.mask.sensitive_key_patterns —
// the single source of truth for sensitive key names used by both masking and lint L-08.
func mergedLintConfig(cfg schema.LintStacksConfig, maskKeyPatterns []string) schema.LintStacksConfig {
	if cfg.MaxImportDepth <= 0 {
		cfg.MaxImportDepth = 3
	}
	if cfg.DRYThresholdPct <= 0 {
		cfg.DRYThresholdPct = 80
	}

	// Build the effective sensitive var patterns using a three-way merge:
	//   1. User-configured lint.stacks.sensitive_var_patterns (highest priority)
	//   2. settings.terminal.mask.sensitive_key_patterns (mask-config patterns)
	//   3. Built-in defaults (always present as a safety net)
	//
	// maskKeyPatterns AUGMENTS the built-in defaults rather than replacing them.
	// This ensures that a user who sets mask patterns does not inadvertently lose
	// the built-in safety net for well-known sensitive names.
	defaults := []string{
		"*password*", "*secret*", "*token*", "*key*",
		"*arn*", "*account_id*", "*role*",
	}
	// mergePatterns deduplicates slices in order, appending unique items from each source.
	mergePatterns := func(sources ...[]string) []string {
		seen := make(map[string]bool)
		var result []string
		for _, src := range sources {
			for _, p := range src {
				if !seen[p] {
					result = append(result, p)
					seen[p] = true
				}
			}
		}
		return result
	}
	// Three-way merge: user patterns → mask patterns → built-in defaults.
	cfg.SensitiveVarPatterns = mergePatterns(cfg.SensitiveVarPatterns, maskKeyPatterns, defaults)

	// Build effective rules by starting with defaults and applying user overrides on top,
	// so that rules not specified by the user still have their default severity.
	defaultRules := map[string]string{
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
	if len(cfg.Rules) > 0 {
		// Merge: start with defaults, then apply user overrides.
		mergedRules := make(map[string]string, len(defaultRules))
		for k, v := range defaultRules {
			mergedRules[k] = v
		}
		for k, v := range cfg.Rules {
			mergedRules[k] = v
		}
		cfg.Rules = mergedRules
	} else {
		cfg.Rules = defaultRules
	}
	return cfg
}

// renderLintText renders findings in human-readable text format using the ui package.
func renderLintText(result *lint.LintResult) {
	if len(result.Findings) == 0 {
		ui.Success("No lint findings.")
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

		// engine.Run already sorts all findings by (severity desc, file, ruleID).
		// Grouping by severity here preserves that order within each group, so
		// no secondary sort is needed.
		for _, f := range findings {
			location := f.File
			if f.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, f.Line)
			}
			var msg string
			if location != "" {
				msg = fmt.Sprintf("[%s] %s  (%s)", f.RuleID, f.Message, location)
			} else {
				msg = fmt.Sprintf("[%s] %s", f.RuleID, f.Message)
			}
			switch sev {
			case lint.SeverityError:
				ui.Error(msg)
			case lint.SeverityWarning:
				ui.Warning(msg)
			default:
				ui.Info(msg)
			}
			if f.FixHint != "" {
				ui.Writeln(fmt.Sprintf("  → %s", f.FixHint))
			}
		}
	}

	ui.Writeln("")
	ui.Writeln(fmt.Sprintf(
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
