package memory

import (
	"regexp"
	"strings"
)

// ParseSections parses a markdown document and extracts sections by ## headers.
func ParseSections(content string) (map[string]*Section, error) {
	sections := make(map[string]*Section)

	// Split by ## headings (level 2 headers).
	lines := strings.Split(content, "\n")

	var currentSection *Section
	var currentKey string
	var contentLines []string

	// Regex to match ## headers.
	headerRegex := regexp.MustCompile(`^##\s+(.+)$`)

	for _, line := range lines {
		// Check if this is a section header.
		if matches := headerRegex.FindStringSubmatch(line); matches != nil {
			// Save previous section if exists.
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				sections[currentKey] = currentSection
			}

			// Start new section.
			sectionTitle := strings.TrimSpace(matches[1])
			currentKey = titleToKey(sectionTitle)
			currentSection = &Section{
				Name:  sectionTitle,
				Order: SectionOrder[currentKey],
			}
			contentLines = []string{}
		} else if currentSection != nil {
			// Accumulate content for current section.
			contentLines = append(contentLines, line)
		}
	}

	// Save final section.
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
		sections[currentKey] = currentSection
	}

	return sections, nil
}

// titleToKey converts a section title to a canonical key.
// Example: "Project Context" → "project_context".
func titleToKey(title string) string {
	// Check if it matches a known title.
	for key, knownTitle := range SectionTitles {
		if strings.EqualFold(title, knownTitle) {
			return key
		}
	}

	// Normalize whitespace: trim and collapse multiple spaces.
	title = strings.TrimSpace(title)
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")

	// Convert to snake_case key.
	key := strings.ToLower(title)
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "&", "and")
	key = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(key, "")

	return key
}

// MergeContent intelligently merges new content into existing section content.
// This preserves manual edits while adding AI-generated updates.
func MergeContent(existing, update string) string {
	// If existing is empty, just use the update.
	if strings.TrimSpace(existing) == "" {
		return update
	}

	// If update is empty, keep existing.
	if strings.TrimSpace(update) == "" {
		return existing
	}

	// Simple append strategy - add update as new paragraph.
	// More sophisticated merging could detect duplicates or conflicts.
	return existing + "\n\n" + update
}

// ExtractSection extracts a specific section from markdown content by key.
func ExtractSection(content, sectionKey string) (string, error) {
	sections, err := ParseSections(content)
	if err != nil {
		return "", err
	}

	if section, ok := sections[sectionKey]; ok {
		return section.Content, nil
	}

	return "", nil
}
