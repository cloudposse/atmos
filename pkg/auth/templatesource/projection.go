package templatesource

import (
	"strings"
	"unicode"
)

// Choose an input-local YAML-printable rune for masked controls. Rune width stays
// unchanged, and a marker in a plain scalar cannot come from another flow entry.
// The usual ASCII input takes one scan; the bounded fallback handles literal markers.
func projectionMarker(input string) rune {
	const first = '\uE000'
	if !strings.ContainsRune(input, first) {
		return first
	}
	used := make(map[rune]bool)
	for _, value := range input {
		used[value] = true
	}
	for candidate := first + 1; candidate <= unicode.MaxRune; candidate++ {
		if printableProjectionRune(candidate) && !used[candidate] {
			return candidate
		}
	}
	return 0
}

func printableProjectionRune(value rune) bool {
	const (
		planeMask              = 0xFFFF
		planeNoncharacterStart = 0xFFFE
		noncharacterStart      = 0xFDD0
		noncharacterEnd        = 0xFDEF
	)
	return value != '\uFEFF' && value&planeMask < planeNoncharacterStart && (value < noncharacterStart || value > noncharacterEnd)
}
