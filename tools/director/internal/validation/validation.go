// Package validation provides post-render validation for SVG outputs.
package validation

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

// Result contains the outcome of validating an SVG file.
type Result struct {
	Scene   string   // Scene name.
	SVGPath string   // Path to the validated SVG.
	Passed  bool     // Whether validation passed.
	Errors  []string // Patterns that matched when they shouldn't (must_not_match violations).
	Missing []string // Patterns that didn't match when they should (must_match violations).
}

// Validator validates SVG outputs against configured patterns.
type Validator struct {
	defaults *scene.ValidationConfig
}

// New creates a new Validator with the given default validation config.
func New(defaults *scene.ValidationConfig) *Validator {
	return &Validator{defaults: defaults}
}

// ValidateSVG checks an SVG file against validation rules.
func (v *Validator) ValidateSVG(svgPath string, sceneValidation *scene.ValidationConfig) (*Result, error) {
	content, err := os.ReadFile(svgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SVG: %w", err)
	}

	// SVGs contain text nodes - extract visible text.
	text := ExtractTextFromSVG(string(content))

	result := &Result{
		SVGPath: svgPath,
		Passed:  true,
	}

	// Get effective patterns (scene overrides defaults).
	mustNotMatch := v.getMustNotMatch(sceneValidation)
	mustMatch := v.getMustMatch(sceneValidation)

	// Check must_not_match patterns (errors if these appear).
	for _, pattern := range mustNotMatch {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid must_not_match pattern %q: %w", pattern, err)
		}
		if re.MatchString(text) {
			result.Passed = false
			// Find the actual match for better error messages.
			match := re.FindString(text)
			result.Errors = append(result.Errors,
				fmt.Sprintf("pattern %q matched: %q", pattern, truncate(match, 60)))
		}
	}

	// Check must_match patterns (errors if these don't appear).
	for _, pattern := range mustMatch {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid must_match pattern %q: %w", pattern, err)
		}
		if !re.MatchString(text) {
			result.Passed = false
			result.Missing = append(result.Missing,
				fmt.Sprintf("pattern %q not found (required)", pattern))
		}
	}

	return result, nil
}

// ExtractTextFromSVG extracts text content from SVG text elements.
// VHS SVGs contain text in <tspan> elements within <text> elements.
func ExtractTextFromSVG(svg string) string {
	// Pattern to extract text content from <tspan> elements.
	// VHS generates SVGs with text like: <tspan ...>content</tspan>
	tspanRe := regexp.MustCompile(`<tspan[^>]*>([^<]*)</tspan>`)
	matches := tspanRe.FindAllStringSubmatch(svg, -1)

	var texts []string
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			texts = append(texts, match[1])
		}
	}

	return strings.Join(texts, "\n")
}

// getMustNotMatch returns the effective must_not_match patterns.
// Scene config takes precedence over defaults (even if empty).
func (v *Validator) getMustNotMatch(sceneConfig *scene.ValidationConfig) []string {
	// If scene explicitly sets must_not_match (even empty), use scene's config.
	if sceneConfig != nil && sceneConfig.MustNotMatch != nil {
		return sceneConfig.MustNotMatch
	}
	// Otherwise use defaults.
	if v.defaults != nil {
		return v.defaults.MustNotMatch
	}
	return nil
}

// getMustMatch returns the effective must_match patterns.
// Scene config takes precedence over defaults (even if empty).
func (v *Validator) getMustMatch(sceneConfig *scene.ValidationConfig) []string {
	if sceneConfig != nil && sceneConfig.MustMatch != nil {
		return sceneConfig.MustMatch
	}
	if v.defaults != nil {
		return v.defaults.MustMatch
	}
	return nil
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
