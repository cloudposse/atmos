package exec

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	log "github.com/charmbracelet/log"
)

// YAMLVersionUpdater provides methods to update versions in YAML files while preserving structure.
type YAMLVersionUpdater struct {
	anchors map[string]string // Track YAML anchors
}

// NewYAMLVersionUpdater creates a new YAML version updater.
func NewYAMLVersionUpdater() *YAMLVersionUpdater {
	return &YAMLVersionUpdater{
		anchors: make(map[string]string),
	}
}

// UpdateVersionsInFile updates versions in a YAML file while preserving structure, anchors, and comments.
func (u *YAMLVersionUpdater) UpdateVersionsInFile(filePath string, updates map[string]string) error {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Update the content
	updatedContent, err := u.UpdateVersionsInContent(data, updates)
	if err != nil {
		return fmt.Errorf("failed to update versions: %w", err)
	}

	// Write the file back
	return os.WriteFile(filePath, updatedContent, 0o644)
}

// UpdateVersionsInContent updates versions in YAML content while preserving structure.
func (u *YAMLVersionUpdater) UpdateVersionsInContent(content []byte, updates map[string]string) ([]byte, error) {
	// First pass: identify all components and their anchors
	componentsToAnchors := u.mapComponentsToAnchors(content)

	scanner := bufio.NewScanner(bytes.NewReader(content))
	var result []string
	var currentComponent string
	var currentAnchor string
	var lastSeenAnchor string

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Track YAML anchors
		if strings.Contains(line, "&") && (strings.Contains(line, ":") || strings.Contains(line, "-")) {
			// This line defines an anchor
			anchorMatch := extractAnchor(line)
			if anchorMatch != "" {
				currentAnchor = anchorMatch
				lastSeenAnchor = anchorMatch
				log.Debug("Found anchor definition", "anchor", currentAnchor)
			}
		}

		// Track anchor references
		if strings.Contains(trimmedLine, "<<: *") {
			// This is an anchor reference
			refMatch := extractAnchorRef(trimmedLine)
			if refMatch != "" {
				currentAnchor = refMatch
				log.Debug("Found anchor reference", "anchor", refMatch)
			}
		}

		// Track component names
		if strings.Contains(trimmedLine, "component:") {
			// Extract component name
			parts := strings.SplitN(trimmedLine, "component:", 2)
			if len(parts) == 2 {
				comp := strings.TrimSpace(parts[1])
				comp = strings.Trim(comp, `"'`)
				currentComponent = comp
				log.Debug("Found component", "component", comp)
			}
		}

		// Check if this is a version line
		if strings.HasPrefix(trimmedLine, "version:") {
			updated := false

			// First check if we have a direct component match
			if currentComponent != "" {
				if newVersion, ok := updates[currentComponent]; ok {
					line = u.replaceVersionInLine(line, newVersion)
					log.Debug("Updated version for component", "component", currentComponent, "version", newVersion)
					updated = true
				}
			}

			// If not updated yet, check if this version belongs to an anchor that a component uses
			if !updated && (currentAnchor != "" || lastSeenAnchor != "") {
				checkAnchor := currentAnchor
				if checkAnchor == "" {
					checkAnchor = lastSeenAnchor
				}

				// Look for components that use this anchor
				for comp, anchors := range componentsToAnchors {
					for _, anchor := range anchors {
						if anchor == checkAnchor {
							if newVersion, ok := updates[comp]; ok {
								line = u.replaceVersionInLine(line, newVersion)
								log.Debug("Updated version in anchor", "anchor", checkAnchor, "component", comp, "version", newVersion)
								updated = true
								break
							}
						}
					}
					if updated {
						break
					}
				}
			}
		}

		// Reset current anchor after we move to a new section
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmedLine != "" && !strings.Contains(line, "<<:") {
			currentAnchor = ""
		}

		result = append(result, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning file: %w", err)
	}

	// Reconstruct the file content
	finalContent := strings.Join(result, "\n")

	// Preserve final newline if original had one
	if len(content) > 0 && content[len(content)-1] == '\n' {
		finalContent += "\n"
	}

	return []byte(finalContent), nil
}

// replaceVersionInLine replaces the version in a line while preserving formatting.
func (u *YAMLVersionUpdater) replaceVersionInLine(line, newVersion string) string {
	// Find the colon position
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return line
	}

	// Extract the indentation
	indentation := ""
	for _, ch := range line {
		if ch == ' ' || ch == '\t' {
			indentation += string(ch)
		} else {
			break
		}
	}

	// Extract everything after the colon
	afterColon := line[colonIdx+1:]

	// Find where the value starts (skip whitespace)
	valueStart := 0
	for i, ch := range afterColon {
		if ch != ' ' && ch != '\t' {
			valueStart = i
			break
		}
	}

	// Extract the value part and any trailing content (like comments)
	valuePart := afterColon[valueStart:]

	// Find if there's an inline comment
	commentIdx := -1
	inQuotes := false
	quoteChar := rune(0)

	for i, ch := range valuePart {
		// Track if we're inside quotes
		if (ch == '"' || ch == '\'') && (i == 0 || valuePart[i-1] != '\\') {
			if !inQuotes {
				inQuotes = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuotes = false
				quoteChar = 0
			}
		}

		// Look for comment start outside of quotes
		if ch == '#' && !inQuotes {
			commentIdx = i
			break
		}
	}

	// Extract current value and trailing comment
	currentValue := valuePart
	trailingComment := ""

	if commentIdx >= 0 {
		currentValue = strings.TrimSpace(valuePart[:commentIdx])
		trailingComment = valuePart[commentIdx:]
	} else {
		currentValue = strings.TrimSpace(valuePart)
	}

	// Preserve quotes if they exist
	formattedVersion := newVersion
	if strings.HasPrefix(currentValue, `"`) && strings.HasSuffix(currentValue, `"`) {
		formattedVersion = fmt.Sprintf(`"%s"`, newVersion)
	} else if strings.HasPrefix(currentValue, `'`) && strings.HasSuffix(currentValue, `'`) {
		formattedVersion = fmt.Sprintf(`'%s'`, newVersion)
	}

	// Reconstruct the line with the same indentation and trailing comment
	if trailingComment != "" {
		// Preserve the spacing before the comment
		spaceBefore := afterColon[:valueStart]
		return fmt.Sprintf("%sversion:%s%s  %s", indentation, spaceBefore, formattedVersion, trailingComment)
	}

	// No trailing comment, just the version
	spaceBefore := afterColon[:valueStart]
	return fmt.Sprintf("%sversion:%s%s", indentation, spaceBefore, formattedVersion)
}

// extractAnchor extracts an anchor name from a line.
func extractAnchor(line string) string {
	// Look for &anchorname pattern
	idx := strings.Index(line, "&")
	if idx == -1 {
		return ""
	}

	// Extract the anchor name
	rest := line[idx+1:]
	anchorName := ""
	for _, ch := range rest {
		if ch == ' ' || ch == '\t' || ch == ':' || ch == '\n' {
			break
		}
		anchorName += string(ch)
	}

	return anchorName
}

// extractAnchorRef extracts an anchor reference from a line.
func extractAnchorRef(line string) string {
	// Look for *anchorname pattern
	idx := strings.Index(line, "*")
	if idx == -1 {
		return ""
	}

	// Extract the anchor reference name
	rest := line[idx+1:]
	refName := ""
	for _, ch := range rest {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == ',' || ch == '}' || ch == ']' {
			break
		}
		refName += string(ch)
	}

	return refName
}

// mapComponentsToAnchors analyzes the YAML to map components to the anchors they use.
func (u *YAMLVersionUpdater) mapComponentsToAnchors(content []byte) map[string][]string {
	componentsToAnchors := make(map[string][]string)
	scanner := bufio.NewScanner(bytes.NewReader(content))

	var currentComponent string
	var currentAnchors []string
	inSourceBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Track when we enter a source block
		if strings.HasPrefix(trimmedLine, "- ") {
			// Save previous component if we have one
			if currentComponent != "" && len(currentAnchors) > 0 {
				componentsToAnchors[currentComponent] = currentAnchors
			}
			// Reset for new block
			currentComponent = ""
			currentAnchors = []string{}
			inSourceBlock = true
		}

		// Track anchor references in current block
		if inSourceBlock && strings.Contains(trimmedLine, "<<: *") {
			refMatch := extractAnchorRef(trimmedLine)
			if refMatch != "" {
				currentAnchors = append(currentAnchors, refMatch)
			}
		}

		// Track component in current block
		if inSourceBlock && strings.Contains(trimmedLine, "component:") {
			parts := strings.SplitN(trimmedLine, "component:", 2)
			if len(parts) == 2 {
				comp := strings.TrimSpace(parts[1])
				comp = strings.Trim(comp, `"'`)
				currentComponent = comp
			}
		}

		// End of block detection
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmedLine != "" && !strings.HasPrefix(trimmedLine, "-") {
			if currentComponent != "" && len(currentAnchors) > 0 {
				componentsToAnchors[currentComponent] = currentAnchors
			}
			inSourceBlock = false
			currentComponent = ""
			currentAnchors = []string{}
		}
	}

	// Save last component if any
	if currentComponent != "" && len(currentAnchors) > 0 {
		componentsToAnchors[currentComponent] = currentAnchors
	}

	return componentsToAnchors
}

// IsTemplatedVersion checks if a version string contains template variables.
func IsTemplatedVersion(version string) bool {
	return strings.Contains(version, "{{") && strings.Contains(version, "}}")
}
