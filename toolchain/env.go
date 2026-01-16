package toolchain

import (
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// SingleQuote is used for shell escaping.
	singleQuote = "'"
	// SingleQuoteEscaped is the escaped form for single-quoted shell strings.
	singleQuoteEscaped = "'\\''"
)

// EmitEnv outputs the PATH entries for installed toolchain binaries in shell-specific format.
// The format parameter specifies the output format (bash, fish, powershell, json, dotenv, github),
// relativeFlag uses relative paths instead of absolute, and outputPath optionally writes to a file.
func EmitEnv(format string, relativeFlag bool, outputPath string) error {
	defer perf.Track(nil, "toolchain.EmitEnv")()

	installer := NewInstaller()

	// Read tool-versions file from the configured path.
	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return errUtils.Build(ErrToolNotFound).
				WithExplanation("no tools configured in tool-versions file").
				WithHint("Run 'atmos toolchain add <tool@version>' to add tools to .tool-versions").
				Err()
		}
		return fmt.Errorf("%w: reading %s: %w", ErrFileOperation, GetToolVersionsFilePath(), err)
	}

	if len(toolVersions.Tools) == 0 {
		return errUtils.Build(ErrToolNotFound).
			WithExplanation("no tools found in .tool-versions file").
			WithHint("Run 'atmos toolchain add <tool@version>' to add tools to .tool-versions").
			Err()
	}

	// Build PATH entries for each tool using the helper from path_helpers.go.
	pathEntries, toolPaths, err := buildPathEntries(toolVersions, installer, relativeFlag)
	if err != nil {
		return err
	}

	// Get current PATH and construct the final PATH value.
	currentPath := getCurrentPath()
	finalPath := constructFinalPath(pathEntries, currentPath)

	// Output based on the requested format.
	return emitEnvOutput(toolPaths, pathEntries, finalPath, format, outputPath)
}

// emitEnvOutput outputs environment variables in the requested format.
func emitEnvOutput(toolPaths []ToolPath, pathEntries []string, finalPath, format, outputPath string) error {
	// If outputPath is specified, append to file instead of stdout.
	if outputPath != "" {
		return appendToFile(outputPath, format, pathEntries, finalPath)
	}

	switch format {
	case "json":
		return emitJSONPath(toolPaths, finalPath)
	case "bash":
		return emitBashEnv(finalPath)
	case "dotenv":
		return emitDotenvEnv(finalPath)
	case "fish":
		return emitFishEnv(finalPath)
	case "powershell":
		return emitPowershellEnv(finalPath)
	case "github":
		return emitGitHubEnv(pathEntries)
	default:
		return emitBashEnv(finalPath)
	}
}

// emitGitHubEnv outputs paths in GitHub Actions GITHUB_PATH format.
// Per GitHub docs: each directory on its own line, appended to $GITHUB_PATH.
// Does NOT include the current PATH - just the tool directories to add.
func emitGitHubEnv(pathEntries []string) error {
	for _, path := range pathEntries {
		if err := data.Writeln(path); err != nil {
			return err
		}
	}
	return nil
}

// formatContentForFile generates the content string for a given format.
func formatContentForFile(format string, pathEntries []string, finalPath string) string {
	switch format {
	case "github":
		return formatGitHubContent(pathEntries)
	case "bash":
		return formatBashContent(finalPath)
	case "dotenv":
		return formatDotenvContent(finalPath)
	case "fish":
		return formatFishContent(finalPath)
	case "powershell":
		return formatPowershellContent(finalPath)
	case "json":
		return fmt.Sprintf(`{"final_path":"%s"}`+"\n", finalPath)
	default:
		return formatBashContent(finalPath)
	}
}

func formatGitHubContent(pathEntries []string) string {
	var sb strings.Builder
	for _, p := range pathEntries {
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatBashContent(finalPath string) string {
	safe := strings.ReplaceAll(finalPath, singleQuote, singleQuoteEscaped)
	return fmt.Sprintf("export PATH='%s'\n", safe)
}

func formatDotenvContent(finalPath string) string {
	safe := strings.ReplaceAll(finalPath, singleQuote, singleQuoteEscaped)
	return fmt.Sprintf("PATH='%s'\n", safe)
}

func formatFishContent(finalPath string) string {
	paths := strings.Split(finalPath, string(os.PathListSeparator))
	var escapedPaths []string
	for _, p := range paths {
		escaped := strings.ReplaceAll(p, singleQuote, "\\'")
		escapedPaths = append(escapedPaths, fmt.Sprintf("'%s'", escaped))
	}
	return fmt.Sprintf("set -gx PATH %s\n", strings.Join(escapedPaths, " "))
}

func formatPowershellContent(finalPath string) string {
	safe := strings.ReplaceAll(finalPath, "\"", "`\"")
	safe = strings.ReplaceAll(safe, "$", "`$")
	return fmt.Sprintf("$env:PATH = \"%s\"\n", safe)
}

// appendToFile appends the environment output to a file (append mode).
func appendToFile(outputPath, format string, pathEntries []string, finalPath string) error {
	content := formatContentForFile(format, pathEntries, finalPath)

	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFileWritePermissions)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", outputPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", outputPath, err)
	}

	_ = ui.Successf("Appended PATH to %s", outputPath)
	return nil
}

// emitBashEnv outputs PATH as bash/zsh export statement.
func emitBashEnv(finalPath string) error {
	// Escape single quotes for safe single-quoted shell literals: ' -> '\''
	safe := strings.ReplaceAll(finalPath, singleQuote, singleQuoteEscaped)
	return data.Writef("export PATH='%s'\n", safe)
}

// emitDotenvEnv outputs PATH in .env format.
func emitDotenvEnv(finalPath string) error {
	// Use the same safe single-quoted escaping as bash output.
	safe := strings.ReplaceAll(finalPath, singleQuote, singleQuoteEscaped)
	return data.Writef("PATH='%s'\n", safe)
}

// emitFishEnv outputs PATH as fish shell set command.
func emitFishEnv(finalPath string) error {
	// Fish uses space-separated paths, not colon-separated.
	// Convert platform-specific separator to spaces.
	paths := strings.Split(finalPath, string(os.PathListSeparator))
	// Escape single quotes for fish: ' -> \'
	var escapedPaths []string
	for _, p := range paths {
		escaped := strings.ReplaceAll(p, singleQuote, "\\'")
		escapedPaths = append(escapedPaths, fmt.Sprintf("'%s'", escaped))
	}
	return data.Writef("set -gx PATH %s\n", strings.Join(escapedPaths, " "))
}

// emitPowershellEnv outputs PATH as PowerShell $env: assignment.
func emitPowershellEnv(finalPath string) error {
	// PowerShell uses semicolon as path separator on Windows, but we use platform-specific.
	// Escape double quotes and dollar signs for PowerShell strings.
	safe := strings.ReplaceAll(finalPath, "\"", "`\"")
	safe = strings.ReplaceAll(safe, "$", "`$")
	return data.Writef("$env:PATH = \"%s\"\n", safe)
}

// emitJSONPath outputs PATH and tool information in JSON format.
func emitJSONPath(toolPaths []ToolPath, finalPath string) error {
	output := struct {
		Tools     []ToolPath `json:"tools"`
		FinalPath string     `json:"final_path"`
		Count     int        `json:"count"`
	}{
		Tools:     toolPaths,
		FinalPath: finalPath,
		Count:     len(toolPaths),
	}
	return data.WriteJSON(output)
}
