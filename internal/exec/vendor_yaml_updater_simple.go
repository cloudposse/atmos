package exec

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SimpleYAMLVersionUpdater provides a simple line-by-line YAML version updater
// that preserves formatting, comments, and anchors without complex AST manipulation.
type SimpleYAMLVersionUpdater struct{}

// NewSimpleYAMLVersionUpdater creates a new simple YAML updater.
func NewSimpleYAMLVersionUpdater() *SimpleYAMLVersionUpdater {
	return &SimpleYAMLVersionUpdater{}
}

// UpdateVersionsInFile updates versions in a YAML file while preserving structure.
func (u *SimpleYAMLVersionUpdater) UpdateVersionsInFile(filePath string, updates map[string]string) error {
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// Update versions in content
	updatedContent, err := u.UpdateVersionsInContent(content, updates)
	if err != nil {
		return fmt.Errorf("failed to update versions: %w", err)
	}

	// Write the file back
	return os.WriteFile(filePath, updatedContent, vendorDefaultFilePermissions)
}

// UpdateVersionsInContent updates component versions in YAML content while preserving structure.
func (u *SimpleYAMLVersionUpdater) UpdateVersionsInContent(content []byte, updates map[string]string) ([]byte, error) {
	if len(updates) == 0 {
		return content, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	var result bytes.Buffer
	processor := &lineProcessor{
		updates:        updates,
		componentRegex: regexp.MustCompile(`^\s*-?\s*component:\s*["']?(\w+)["']?\s*(?:#.*)?$`),
		versionRegex:   regexp.MustCompile(`^(\s*version:\s*)["']?([^"'\s]+)["']?(\s*(?:#.*)?)$`),
	}

	for scanner.Scan() {
		line := scanner.Text()
		processedLine := processor.processLine(line)
		result.WriteString(processedLine)
		result.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading content: %w", err)
	}

	return u.trimTrailingNewline(content, result.Bytes()), nil
}

// lineProcessor handles the processing of individual lines.
type lineProcessor struct {
	currentComponent string
	updates          map[string]string
	componentRegex   *regexp.Regexp
	versionRegex     *regexp.Regexp
}

// processLine processes a single line of YAML.
func (p *lineProcessor) processLine(line string) string {
	// Check if this line defines a component
	if matches := p.componentRegex.FindStringSubmatch(line); matches != nil {
		p.currentComponent = matches[1]
		return line
	}

	// Try to update version if we have a current component
	if updatedLine := p.tryUpdateVersion(line); updatedLine != "" {
		return updatedLine
	}

	// Check if we're leaving the current component context
	p.checkComponentContext(line)

	return line
}

// tryUpdateVersion attempts to update the version in the line if applicable.
func (p *lineProcessor) tryUpdateVersion(line string) string {
	if p.currentComponent == "" {
		return ""
	}

	newVersion, exists := p.updates[p.currentComponent]
	if !exists {
		return ""
	}

	matches := p.versionRegex.FindStringSubmatch(line)
	if matches == nil {
		return ""
	}

	// Replace the version while preserving indentation and comments
	p.currentComponent = "" // Reset after updating
	return matches[1] + `"` + newVersion + `"` + matches[3]
}

// checkComponentContext checks if we're leaving the current component context.
func (p *lineProcessor) checkComponentContext(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}

	// If line starts at the same or lower indentation as component, we're leaving the component
	if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
		p.currentComponent = ""
	}
}

// trimTrailingNewline removes the trailing newline if the original didn't have one.
func (u *SimpleYAMLVersionUpdater) trimTrailingNewline(original, result []byte) []byte {
	if len(original) > 0 && original[len(original)-1] != '\n' && len(result) > 0 && result[len(result)-1] == '\n' {
		return result[:len(result)-1]
	}
	return result
}
