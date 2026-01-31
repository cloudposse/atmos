package builtin

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// Skills embeds all built-in skill directories following the Agent Skills standard.
// Each skill is a directory containing a SKILL.md file.
//
//go:embed */SKILL.md
var Skills embed.FS

// Read reads a prompt file from the embedded filesystem.
// The file should be in SKILL.md format with YAML frontmatter.
// This function returns only the markdown body (after the frontmatter).
func Read(filename string) (string, error) {
	content, err := fs.ReadFile(Skills, filename)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded prompt file %q: %w", filename, err)
	}

	// Parse SKILL.md format: extract content after YAML frontmatter.
	body, err := extractMarkdownBody(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse SKILL.md format in %q: %w", filename, err)
	}

	return body, nil
}

// extractMarkdownBody extracts the markdown body from a SKILL.md file.
// SKILL.md files have YAML frontmatter between --- delimiters.
func extractMarkdownBody(content string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	inFrontmatter := false
	frontmatterEnded := false
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Check for frontmatter delimiter.
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter && lineNum == 1 {
				// Start of frontmatter.
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				// End of frontmatter.
				inFrontmatter = false
				frontmatterEnded = true
				continue
			}
		}

		// Skip lines in frontmatter.
		if inFrontmatter {
			continue
		}

		// Collect lines after frontmatter.
		if frontmatterEnded {
			lines = append(lines, line)
		} else {
			// No frontmatter, return content as-is.
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning content: %w", err)
	}

	// Join lines and trim leading/trailing whitespace.
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

// ReadMetadata reads only the YAML frontmatter from a SKILL.md file.
// This is useful for loading skill metadata without the full prompt.
func ReadMetadata(filename string) (string, error) {
	content, err := fs.ReadFile(Skills, filename)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded prompt file %q: %w", filename, err)
	}

	frontmatter, err := extractFrontmatter(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to extract frontmatter from %q: %w", filename, err)
	}

	return frontmatter, nil
}

// extractFrontmatter extracts the YAML frontmatter from a SKILL.md file.
func extractFrontmatter(content string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	inFrontmatter := false
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Check for frontmatter delimiter.
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter && lineNum == 1 {
				// Start of frontmatter.
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				// End of frontmatter.
				break
			}
		}

		// Collect lines in frontmatter.
		if inFrontmatter {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning content: %w", err)
	}

	return strings.Join(lines, "\n"), nil
}

// List returns all available prompt filenames.
func List() ([]string, error) {
	var files []string

	err := fs.WalkDir(Skills, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-.md files.
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}

		// Only include SKILL.md files in subdirectories.
		if strings.HasSuffix(path, "/SKILL.md") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded prompts: %w", err)
	}

	return files, nil
}
