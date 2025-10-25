package atmos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// EditOperation represents the type of edit operation.
type EditOperation string

const (
	// OperationSearchReplace finds and replaces text in the file.
	OperationSearchReplace EditOperation = "search_replace"
	// OperationInsertLine inserts a line at a specific position.
	OperationInsertLine EditOperation = "insert_line"
	// OperationDeleteLines deletes a range of lines.
	OperationDeleteLines EditOperation = "delete_lines"
	// OperationAppend appends text to the end of the file.
	OperationAppend EditOperation = "append"
)

// EditFileTool performs incremental file editing operations.
type EditFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewEditFileTool creates a new file editing tool.
func NewEditFileTool(atmosConfig *schema.AtmosConfiguration) *EditFileTool {
	return &EditFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *EditFileTool) Name() string {
	return "edit_file"
}

// Description returns the tool description.
func (t *EditFileTool) Description() string {
	return "Edit a file with surgical precision using incremental operations: search_replace (find and replace text), insert_line (insert at line number), delete_lines (remove line range), or append (add to end). More token-efficient than rewriting entire files. Use this for targeted changes to configuration files, code files, or any text file."
}

// Parameters returns the tool parameters.
func (t *EditFileTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Path to the file to edit. Can be relative to Atmos base path or absolute.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "operation",
			Description: "Edit operation: 'search_replace' (find and replace text), 'insert_line' (insert at line number), 'delete_lines' (remove line range), 'append' (add to end).",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "search",
			Description: "For search_replace: the text to find. Must be exact match.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "replace",
			Description: "For search_replace: the replacement text.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "line_number",
			Description: "For insert_line: the line number (1-based) where to insert.",
			Type:        tools.ParamTypeInt,
			Required:    false,
		},
		{
			Name:        "start_line",
			Description: "For delete_lines: the starting line number (1-based, inclusive).",
			Type:        tools.ParamTypeInt,
			Required:    false,
		},
		{
			Name:        "end_line",
			Description: "For delete_lines: the ending line number (1-based, inclusive).",
			Type:        tools.ParamTypeInt,
			Required:    false,
		},
		{
			Name:        "content",
			Description: "For insert_line or append: the text content to insert/append.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute performs the file editing operation.
func (t *EditFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract file_path parameter.
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: file_path", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// Resolve to absolute path.
	absolutePath := filePath
	if !filepath.IsAbs(filePath) {
		absolutePath = filepath.Join(t.atmosConfig.BasePath, filePath)
	}

	// Clean the path to prevent traversal attacks.
	cleanPath := filepath.Clean(absolutePath)

	// Security check: ensure the path is within the Atmos base path.
	cleanBase := filepath.Clean(t.atmosConfig.BasePath)
	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("access denied: file must be within Atmos base path"),
		}, nil
	}

	// Extract operation parameter.
	operationStr, ok := params["operation"].(string)
	if !ok || operationStr == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: operation", errUtils.ErrAIToolParameterRequired),
		}, nil
	}
	operation := EditOperation(operationStr)

	// Check if file exists.
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAIFileNotFound, filePath),
		}, nil
	}

	// Read file content.
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to read file %s: %w", filePath, err),
		}, nil
	}

	// Perform operation.
	var newContent string
	var operationDesc string

	switch operation {
	case OperationSearchReplace:
		newContent, operationDesc, err = t.searchReplace(string(content), params)
	case OperationInsertLine:
		newContent, operationDesc, err = t.insertLine(string(content), params)
	case OperationDeleteLines:
		newContent, operationDesc, err = t.deleteLines(string(content), params)
	case OperationAppend:
		newContent, operationDesc, err = t.appendContent(string(content), params)
	default:
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("unknown operation: %s", operation),
		}, nil
	}

	if err != nil {
		//nolint:nilerr // Tool errors are returned in Result.Error, not in err return value.
		return &tools.Result{
			Success: false,
			Error:   err,
		}, nil
	}

	// Write modified content back to file.
	if err := os.WriteFile(cleanPath, []byte(newContent), filePermissions); err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to write file %s: %w", filePath, err),
		}, nil
	}

	log.Debug(fmt.Sprintf("Edited file: %s - %s", filePath, operationDesc))

	return &tools.Result{
		Success: true,
		Output:  fmt.Sprintf("Successfully edited %s: %s", filePath, operationDesc),
		Data: map[string]interface{}{
			"file_path":  filePath,
			"operation":  string(operation),
			"old_size":   len(content),
			"new_size":   len(newContent),
			"size_delta": len(newContent) - len(content),
		},
	}, nil
}

// searchReplace performs a search and replace operation.
func (t *EditFileTool) searchReplace(content string, params map[string]interface{}) (string, string, error) {
	search, ok := params["search"].(string)
	if !ok || search == "" {
		return "", "", fmt.Errorf("%w: search", errUtils.ErrAIToolParameterRequired)
	}

	replace, ok := params["replace"].(string)
	if !ok {
		return "", "", fmt.Errorf("%w: replace", errUtils.ErrAIToolParameterRequired)
	}

	if !strings.Contains(content, search) {
		return "", "", fmt.Errorf("search text not found in file")
	}

	newContent := strings.ReplaceAll(content, search, replace)
	count := strings.Count(content, search)

	return newContent, fmt.Sprintf("replaced %d occurrence(s) of text", count), nil
}

// insertLine inserts content at a specific line number.
func (t *EditFileTool) insertLine(content string, params map[string]interface{}) (string, string, error) {
	lineNum, ok := params["line_number"].(float64)
	if !ok {
		return "", "", fmt.Errorf("%w: line_number", errUtils.ErrAIToolParameterRequired)
	}

	insertContent, ok := params["content"].(string)
	if !ok {
		return "", "", fmt.Errorf("%w: content", errUtils.ErrAIToolParameterRequired)
	}

	lines := strings.Split(content, "\n")
	lineIdx := int(lineNum) - 1 // Convert to 0-based index.

	if lineIdx < 0 || lineIdx > len(lines) {
		return "", "", fmt.Errorf("line number %d out of range (file has %d lines)", int(lineNum), len(lines))
	}

	// Insert the new line.
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:lineIdx]...)
	newLines = append(newLines, insertContent)
	newLines = append(newLines, lines[lineIdx:]...)

	return strings.Join(newLines, "\n"), fmt.Sprintf("inserted line at position %d", int(lineNum)), nil
}

// deleteLines deletes a range of lines.
func (t *EditFileTool) deleteLines(content string, params map[string]interface{}) (string, string, error) {
	startLine, ok := params["start_line"].(float64)
	if !ok {
		return "", "", fmt.Errorf("%w: start_line", errUtils.ErrAIToolParameterRequired)
	}

	endLine, ok := params["end_line"].(float64)
	if !ok {
		return "", "", fmt.Errorf("%w: end_line", errUtils.ErrAIToolParameterRequired)
	}

	lines := strings.Split(content, "\n")
	startIdx := int(startLine) - 1 // Convert to 0-based index (inclusive).
	endIdx := int(endLine) - 1     // Convert to 0-based index (inclusive).

	if startIdx < 0 || endIdx >= len(lines) || startIdx > endIdx {
		return "", "", fmt.Errorf("invalid line range %d-%d (file has %d lines)", int(startLine), int(endLine), len(lines))
	}

	// Delete the lines (endIdx is inclusive, so we keep lines after endIdx+1).
	newLines := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	newLines = append(newLines, lines[:startIdx]...)
	newLines = append(newLines, lines[endIdx+1:]...)

	count := endIdx - startIdx + 1
	return strings.Join(newLines, "\n"), fmt.Sprintf("deleted %d line(s) from %d to %d", count, int(startLine), int(endLine)), nil
}

// appendContent appends content to the end of the file.
func (t *EditFileTool) appendContent(content string, params map[string]interface{}) (string, string, error) {
	appendContent, ok := params["content"].(string)
	if !ok {
		return "", "", fmt.Errorf("%w: content", errUtils.ErrAIToolParameterRequired)
	}

	// Ensure file ends with newline before appending.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	newContent := content + appendContent
	return newContent, "appended content to end of file", nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *EditFileTool) RequiresPermission() bool {
	return true // File modification requires user confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *EditFileTool) IsRestricted() bool {
	return false
}
