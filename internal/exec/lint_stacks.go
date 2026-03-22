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
	// When a stack filter is active, also narrow RawStackConfigs to the files that
	// feed into that stack so that L-03, L-05, and L-07 are fully scoped.
	if stackFilter != "" {
		filteredStacks := make(map[string]any)
		for name, v := range stacksMap {
			if name == stackFilter {
				filteredStacks[name] = v
			}
		}
		stacksMap = filteredStacks

		// Narrow rawStackConfigs to files whose normalized name matches the filter.
		// Because Atmos logical stack names typically correspond 1:1 with YAML file
		// stems (e.g. "plat-ue2-prod" ↔ "stacks/deploy/plat-ue2-prod.yaml"), we
		// include a raw config entry when its file stem (extension + base path stripped)
		// matches any filtered stack name.
		filteredRaw := make(map[string]map[string]any)
		for filePath, rawConfig := range rawStackConfigs {
			// Derive the logical name from the file path by stripping the base path
			// prefix and extension, mirroring how Atmos registers stacks.
			base := filepath.Base(filePath)
			for _, ext := range []string{".yaml", ".yml"} {
				if strings.HasSuffix(base, ext) {
					base = base[:len(base)-len(ext)]
					break
				}
			}
			if _, ok := filteredStacks[base]; ok {
				filteredRaw[filePath] = rawConfig
			}
		}
		// Fall back to the full set if matching produced no results (e.g. when stack
		// names encode environment + region context that doesn't match the bare filename).
		if len(filteredRaw) > 0 {
			rawStackConfigs = filteredRaw
		}
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

	// When a stack filter is active, scope AllStackFiles to only the files that are
	// reachable from the filtered root stack via the import graph.  This prevents
	// L-07 from reporting orphaned files that belong to other stacks in the repo —
	// which would be noise, not actionable findings for the targeted stack.
	if stackFilter != "" && len(importGraph) > 0 {
		allStackFiles = scopeStackFiles(allStackFiles, rawStackConfigs, importGraph, atmosConfig.StacksBaseAbsolutePath)
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
						// Respect the optional "enabled" field on map-form imports.
						// An import with enabled: false is disabled and should not be
						// counted as referenced by L-03 or L-07.
						if en, ok := s["enabled"].(bool); ok && !en {
							continue
						}
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
// Duplicate results from overlapping patterns (e.g. "catalog/*" and "catalog/*.yaml"
// both matching the same file) are automatically deduplicated.
func expandGlobImports(imports []string, basePath string) []string {
	if basePath == "" {
		return imports
	}
	// seen tracks already-added absolute paths to prevent duplicates when multiple
	// patterns (e.g. *.yaml and *.yml) expand to the same file.
	seen := make(map[string]bool)
	result := make([]string, 0, len(imports))
	for _, imp := range imports {
		if !strings.ContainsAny(imp, "*?[") {
			// Not a glob — pass through unchanged (duplicates here are kept to
			// preserve the original import list semantics).
			result = append(result, imp)
			continue
		}
		// Build the set of glob patterns to try. If the import already ends with
		// ".yaml" or ".yml" (e.g. "catalog/*.yaml"), only match as-is. Otherwise
		// try both extensions and the bare pattern so that extension-less imports
		// like "catalog/**/*" are also expanded correctly.
		var patterns []string
		ext := strings.ToLower(filepath.Ext(imp))
		if ext == ".yaml" || ext == ".yml" {
			patterns = []string{filepath.Join(basePath, imp)}
		} else {
			patterns = []string{
				filepath.Join(basePath, imp+".yaml"),
				filepath.Join(basePath, imp+".yml"),
				filepath.Join(basePath, imp),
			}
		}
		var matched []string
		for _, pattern := range patterns {
			hits, err := filepath.Glob(pattern)
			if err != nil {
				// Malformed pattern (e.g. unclosed bracket) — log and skip.
				log.Debug("Skipping malformed glob import pattern", "pattern", pattern, "error", err)
				continue
			}
			for _, h := range hits {
				if !seen[h] {
					seen[h] = true
					matched = append(matched, h)
				}
			}
		}
		if len(matched) == 0 {
			// No expansion — keep the literal so callers see the original intent.
			// Note: literal globs with no matches are NOT added to the seen map so
			// they don't prevent a later real file from being recorded.
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

// scopeStackFiles narrows allStackFiles to only those reachable from the root
// stack files in rawStackConfigs via a BFS traversal of importGraph.  This ensures
// that when a --stack filter is active, L-07 only flags orphans that are actually
// in scope — preventing false positives from other stacks in the same repo.
//
// importGraph keys are absolute paths; values are the import paths recorded for
// that file (may be absolute or relative, with or without YAML extension).
func scopeStackFiles(allStackFiles []string, rawStackConfigs map[string]map[string]any, importGraph map[string][]string, basePath string) []string {
	// Build the reachable set via BFS starting from the root files (rawStackConfigs keys).
	reachable := make(map[string]bool)

	queue := make([]string, 0, len(rawStackConfigs))
	for k := range rawStackConfigs {
		rn := rulesRelNorm(k, basePath)
		if !reachable[rn] {
			reachable[rn] = true
			queue = append(queue, k) // use absolute path for importGraph lookup
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, imp := range importGraph[cur] {
			rn := rulesRelNorm(imp, basePath)
			if !reachable[rn] {
				reachable[rn] = true
				// Reconstruct the absolute path so we can look it up in importGraph.
				abs := imp
				if !filepath.IsAbs(abs) && basePath != "" {
					abs = filepath.Join(basePath, abs)
				}
				queue = append(queue, abs)
			}
		}
	}

	// Keep only files whose normalized name is in the reachable set.
	result := make([]string, 0, len(allStackFiles))
	for _, f := range allStackFiles {
		if reachable[rulesRelNorm(f, basePath)] {
			result = append(result, f)
		}
	}
	return result
}

// rulesRelNorm mirrors the relNorm logic used in L-07 (pkg/lint/rules/l07_orphaned_file.go)
// so that paths are compared consistently in the exec package without importing the rules package.
// It strips the base path prefix and YAML extension for uniform comparison.
func rulesRelNorm(path, basePath string) string {
	if filepath.IsAbs(path) && basePath != "" {
		if rel, err := filepath.Rel(basePath, path); err == nil {
			path = rel
		}
	}
	// Strip YAML extensions (same as normalizeForComparison in l07).
	for _, ext := range []string{".yaml", ".yml"} {
		if len(path) > len(ext) && strings.HasSuffix(path, ext) {
			path = path[:len(path)-len(ext)]
			break
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
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
	//
	// Note: patterns like "*arn*", "*account_id*", and "*role*" are intentionally
	// NOT included in the defaults because they match ubiquitous infrastructure
	// variables (e.g. "iam_role", "account_id", "region_arn") and would produce
	// excessive noise in typical stacks. Add them to your atmos.yaml as opt-in:
	//   lint:
	//     stacks:
	//       sensitive_var_patterns:
	//         - "*arn*"
	//         - "*account_id*"
	//         - "*role*"
	defaults := []string{
		"*password*", "*secret*", "*token*", "*key*",
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
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("rendering lint output as JSON: %w", err)
	}
	return nil
}
