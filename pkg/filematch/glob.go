package filematch

import (
	"regexp"
	"strings"

	"github.com/gobwas/glob"
)

// defaultGlobCompiler implements GlobCompiler using gobwas/glob.
type defaultGlobCompiler struct{}

func NewDefaultGlobCompiler() globCompiler {
	return &defaultGlobCompiler{}
}

// Compile expands brace patterns (e.g., *.{yml,yaml}) and compiles multiple glob patterns.
func (c *defaultGlobCompiler) Compile(pattern string) (compiledGlob, error) {
	expandedPatterns := c.expandBraces(pattern)

	var compiledGlobs []glob.Glob
	for _, p := range expandedPatterns {
		g, err := glob.Compile(p)
		if err != nil {
			return nil, err
		}
		compiledGlobs = append(compiledGlobs, g)
	}

	return &defaultGlob{compiledGlobs}, nil
}

// expandBraces expands patterns like "*.y{ml,aml}" into multiple patterns ["*.yml", "*.yaml"]
// If no braces are found, it returns the original pattern in a slice.
func (c *defaultGlobCompiler) expandBraces(pattern string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindStringSubmatch(pattern)

	if len(matches) == 0 {
		return []string{pattern} // No braces found, return original pattern
	}

	options := strings.Split(matches[1], ",")
	var expanded []string

	for _, opt := range options {
		expanded = append(expanded, re.ReplaceAllString(pattern, opt))
	}

	return expanded
}

// defaultGlob now holds multiple compiled globs.
type defaultGlob struct {
	globs []glob.Glob
}

// Match checks if any compiled glob matches the input string.
func (g *defaultGlob) Match(s string) bool {
	for _, glob := range g.globs {
		if glob.Match(s) {
			return true
		}
	}
	return false
}
