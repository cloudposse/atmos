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

// Execute searches for the pattern and returns matching results.
func (t *SearchFilesTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract pattern parameter.
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: pattern", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// Extract optional path parameter.
	searchPath := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchPath = p
	}

	// Extract optional file_pattern parameter.
	filePattern := "*"
	if fp, ok := params["file_pattern"].(string); ok && fp != "" {
		filePattern = fp
	}

	// Extract optional case_sensitive parameter.
	caseSensitive := false
	if cs, ok := params["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	log.Debug(fmt.Sprintf("Searching for pattern '%s' in path '%s' with file pattern '%s'", pattern, searchPath, filePattern))

	// Resolve search path.
	absolutePath := filepath.Join(t.atmosConfig.BasePath, searchPath)
	cleanPath := filepath.Clean(absolutePath)

	// Security check: ensure search path is within Atmos base path.
	cleanBase := filepath.Clean(t.atmosConfig.BasePath)
	// Allow if path is exactly the base path OR starts with base path followed by separator.
	isWithinBase := cleanPath == cleanBase || strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator))
	if !isWithinBase {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("access denied: search path '%s' must be within Atmos base path '%s'", cleanPath, cleanBase),
		}, nil
	}

	// Compile regex pattern.
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	regex, err := regexp.Compile(flags + pattern)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("invalid pattern: %w", err),
		}, nil
	}

	// Search for matches.
	matches := []string{}
	matchCount := 0

	err = filepath.Walk(cleanPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Skip files with errors and continue walking.
		}

		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// Check file pattern.
		if filePattern != "*" {
			matched, _ := filepath.Match(filePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		// Read file content.
		content, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // Skip files we can't read and continue walking.
		}

		// Search for pattern in file.
		lines := strings.Split(string(content), "\n")
		hasMatch := false
		fileMatches := []string{}

		for lineNum, line := range lines {
			if regex.MatchString(line) {
				matchCount++
				hasMatch = true
				fileMatches = append(fileMatches, fmt.Sprintf("  Line %d: %s", lineNum+1, strings.TrimSpace(line)))
			}
		}

		if hasMatch {
			// Get relative path from base.
			relPath, _ := filepath.Rel(t.atmosConfig.BasePath, path)
			matches = append(matches, fmt.Sprintf("\n%s:\n%s", relPath, strings.Join(fileMatches, "\n")))
		}

		return nil
	})
	if err != nil {
		//nolint:nilerr // Tool errors are returned in Result.Error, not in err return value.
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("search failed: %w", err),
		}, nil
	}

	// Format output.
	var output string
	if len(matches) == 0 {
		output = fmt.Sprintf("No matches found for pattern '%s' in path '%s'", pattern, searchPath)
	} else {
		output = fmt.Sprintf("Found %d matches in %d files for pattern '%s':\n%s",
			matchCount, len(matches), pattern, strings.Join(matches, "\n"))
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"pattern":      pattern,
			"path":         searchPath,
			"file_pattern": filePattern,
			"match_count":  matchCount,
			"file_count":   len(matches),
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *SearchFilesTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *SearchFilesTool) IsRestricted() bool {
	return false
}
