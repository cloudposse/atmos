package atmos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SearchFilesTool searches for patterns in files.
type SearchFilesTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewSearchFilesTool creates a new file search tool.
func NewSearchFilesTool(atmosConfig *schema.AtmosConfiguration) *SearchFilesTool {
	return &SearchFilesTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *SearchFilesTool) Name() string {
	return "search_files"
}

// Description returns the tool description.
func (t *SearchFilesTool) Description() string {
	return "Search for a text pattern or regex across files in the Atmos repository. Returns file paths and matching lines with line numbers. Use this to find components using specific modules, stacks with particular configurations, or any text pattern. Examples: search for 'module \"vpc\"', 'terraform_version', 'backend_type: s3'."
}

// Parameters returns the tool parameters.
func (t *SearchFilesTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "pattern",
			Description: "The text pattern or regular expression to search for (e.g., 'module \"vpc\"', 'terraform_version', 'backend_type').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "path",
			Description: "The path to search in, relative to Atmos base path. Use '.' for entire repository, 'components/terraform' for components, 'stacks' for stacks. Default is '.' (entire repository).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "file_pattern",
			Description: "File pattern to filter (e.g., '*.yaml', '*.tf', '*.hcl'). Default is '*' (all files).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "case_sensitive",
			Description: "Whether the search should be case-sensitive. Default is false.",
			Type:        tools.ParamTypeBool,
			Required:    false,
		},
	}
}

// searchParams holds the extracted and validated search parameters.
type searchParams struct {
	pattern       string
	searchPath    string
	filePattern   string
	caseSensitive bool
}

// extractSearchParams extracts and validates search parameters from the params map.
func extractSearchParams(params map[string]interface{}) (*searchParams, *tools.Result) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return nil, &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: pattern", errUtils.ErrAIToolParameterRequired),
		}
	}

	searchPath := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchPath = p
	}

	filePattern := "*"
	if fp, ok := params["file_pattern"].(string); ok && fp != "" {
		filePattern = fp
	}

	caseSensitive := false
	if cs, ok := params["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	return &searchParams{
		pattern:       pattern,
		searchPath:    searchPath,
		filePattern:   filePattern,
		caseSensitive: caseSensitive,
	}, nil
}

// Execute searches for the pattern and returns matching results.
func (t *SearchFilesTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	sp, errResult := extractSearchParams(params)
	if errResult != nil {
		return errResult, nil
	}

	log.Debugf("Searching for pattern '%s' in path '%s' with file pattern '%s'", sp.pattern, sp.searchPath, sp.filePattern)

	// Resolve and validate search path.
	absolutePath := filepath.Join(t.atmosConfig.BasePath, sp.searchPath)
	cleanPath := filepath.Clean(absolutePath)
	cleanBase := filepath.Clean(t.atmosConfig.BasePath)

	isWithinBase := cleanPath == cleanBase || strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator))
	if !isWithinBase {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: '%s' is outside '%s'", errUtils.ErrAIAccessDeniedSearchPath, cleanPath, cleanBase),
		}, nil
	}

	// Compile regex pattern.
	flags := ""
	if !sp.caseSensitive {
		flags = "(?i)"
	}
	regex, err := regexp.Compile(flags + sp.pattern)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("invalid pattern: %w", err),
		}, nil
	}

	// Execute the search.
	matches, matchCount, err := t.searchInFiles(cleanPath, sp.filePattern, regex)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("search failed: %w", err),
		}, nil
	}

	return buildSearchResult(sp, matches, matchCount), nil
}

// buildSearchResult formats search results into a tools.Result.
func buildSearchResult(sp *searchParams, matches []string, matchCount int) *tools.Result {
	var output string
	if len(matches) == 0 {
		output = fmt.Sprintf("No matches found for pattern '%s' in path '%s'", sp.pattern, sp.searchPath)
	} else {
		output = fmt.Sprintf("Found %d matches in %d files for pattern '%s':\n%s",
			matchCount, len(matches), sp.pattern, strings.Join(matches, "\n"))
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"pattern":      sp.pattern,
			"path":         sp.searchPath,
			"file_pattern": sp.filePattern,
			"match_count":  matchCount,
			"file_count":   len(matches),
		},
	}
}

// searchInFiles walks the directory tree and searches for regex matches in files.
func (t *SearchFilesTool) searchInFiles(rootPath, filePattern string, regex *regexp.Regexp) ([]string, int, error) {
	var matches []string
	matchCount := 0

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // Skip files with errors and directories.
		}

		if filePattern != "*" {
			matched, _ := filepath.Match(filePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // Skip files we can't read and continue walking.
		}

		lines := strings.Split(string(content), "\n")
		var fileMatches []string
		for lineNum, line := range lines {
			if regex.MatchString(line) {
				matchCount++
				fileMatches = append(fileMatches, fmt.Sprintf("  Line %d: %s", lineNum+1, strings.TrimSpace(line)))
			}
		}

		if len(fileMatches) > 0 {
			relPath, _ := filepath.Rel(t.atmosConfig.BasePath, path)
			matches = append(matches, fmt.Sprintf("\n%s:\n%s", relPath, strings.Join(fileMatches, "\n")))
		}

		return nil
	})

	return matches, matchCount, err
}

// RequiresPermission returns true if this tool needs permission.
func (t *SearchFilesTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *SearchFilesTool) IsRestricted() bool {
	return false
}
