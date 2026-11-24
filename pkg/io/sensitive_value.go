package io

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cloudposse/atmos/pkg/perf"
)

// minMaskableRunes is the shortest text registered for masking from a declared-sensitive value. Masking is
// a literal replacement in all output, so a shorter text, such as "app" or "3", would also hide unrelated
// words and digits ("known after apply", timings) while protecting almost nothing.
const minMaskableRunes = 4

// trivialSecret reports whether a text is too short or too common to mask without hiding unrelated output:
// fewer than minMaskableRunes characters once trimmed, no letter or digit (a closing brace), or a boolean
// or null literal.
func trivialSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	if utf8.RuneCountInString(trimmed) < minMaskableRunes {
		return true
	}
	switch strings.ToLower(trimmed) {
	case "true", "false", "null":
		return true
	}
	return !strings.ContainsFunc(trimmed, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
}

// structuralLine reports whether a line of a sensitive text only opens a block, such as `terraform {` or
// `backend "s3" {`. It names structure, not a value, and appears in unrelated output too.
func structuralLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasSuffix(trimmed, "{") && !strings.Contains(trimmed, "=")
}

// RegisterSensitiveValue registers the parts of a value a provider declared sensitive, such as a sensitive
// Terragrunt input decoded from JSON. Every distinctive scalar is registered with its encodings, and the
// string members of a JSON-encoded string are registered too. Object keys are structure, not values, and
// trivial scalars (see trivialSecret) are skipped, so masking never spills into unrelated output.
func RegisterSensitiveValue(v any) {
	defer perf.Track(nil, "io.RegisterSensitiveValue")()

	registerSensitiveValue(v)
}

func registerSensitiveValue(v any) {
	switch t := v.(type) {
	case nil:
		return
	case map[string]any:
		for _, child := range t {
			registerSensitiveValue(child)
		}
	case []any:
		for _, child := range t {
			registerSensitiveValue(child)
		}
	case string:
		registerSensitiveString(t)
	default:
		if text := fmt.Sprintf("%v", t); !trivialSecret(text) {
			RegisterSecret(text)
		}
	}
}

func registerSensitiveString(value string) {
	if strings.ContainsAny(value, "\r\n") {
		registerSensitiveText(value)
	} else if !trivialSecret(value) {
		RegisterSecret(value)
	}
	var structured any
	if json.Unmarshal([]byte(value), &structured) != nil {
		return
	}
	switch structured.(type) {
	case map[string]any, []any:
		registerSensitiveValue(structured)
	}
}

// RegisterSensitiveText registers a sensitive multi-line text, such as a generated file that can hold
// credentials. The whole text is masked, and so is each distinctive line, for output that shows only part
// of it. Trivial lines and lines that only open a block are skipped: a closing brace or `terraform {` also
// appears in unrelated output.
func RegisterSensitiveText(text string) {
	defer perf.Track(nil, "io.RegisterSensitiveText")()

	registerSensitiveText(text)
}

func registerSensitiveText(text string) {
	if trivialSecret(text) {
		return
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.Contains(normalized, "\n") {
		RegisterSecret(text)
		return
	}
	RegisterValue(text)
	if normalized != text {
		RegisterValue(normalized)
	}
	for _, line := range strings.Split(normalized, "\n") {
		// The trimmed line also matches the line shown with a different indentation.
		if !trivialSecret(line) && !structuralLine(line) {
			RegisterSecret(strings.TrimSpace(line))
		}
	}
}
