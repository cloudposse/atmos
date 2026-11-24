package io

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cloudposse/atmos/pkg/perf"
)

// minMaskableRunes is the shortest text registered for masking from a declared-sensitive value. Masking is
// a literal replacement in all output, so a shorter text, such as "app" or "3", would also hide unrelated
// words and digits ("known after apply", timings) while protecting almost nothing.
const minMaskableRunes = 4

// trivialSecret reports whether a value is too short or too common to mask without hiding unrelated output:
// fewer than minMaskableRunes characters once trimmed (a closing brace, "app", a small number), or a boolean
// or null literal. A longer value made only of symbols, such as a password, is still a secret.
func trivialSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	if utf8.RuneCountInString(trimmed) < minMaskableRunes {
		return true
	}
	switch strings.ToLower(trimmed) {
	case "true", "false", "null":
		return true
	}
	return false
}

// structuralLine reports whether a line of a sensitive text is structure rather than a value: it only opens a
// block, such as `terraform {` or `backend "s3" {`, or it has no letter or digit, such as `}},` or a `-----`
// separator. Such a line appears in unrelated output too, so it is not masked on its own; the complete text
// that contains it still is.
func structuralLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.ContainsFunc(trimmed, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return true
	}
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

// registerEncodedText registers a complete multi-line text and the encoded forms the masker registers for a
// secret (base64 in every variant, URL query escaping, and JSON escaping), without registering its lines.
func registerEncodedText(text string) {
	RegisterValue(text)
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		RegisterValue(encoding.EncodeToString([]byte(text)))
	}
	RegisterValue(url.QueryEscape(text))
	if escaped, err := json.Marshal(text); err == nil && len(escaped) > 2 {
		RegisterValue(string(escaped[1 : len(escaped)-1]))
	}
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
	registerEncodedText(text)
	if normalized != text {
		registerEncodedText(normalized)
	}
	for _, line := range strings.Split(normalized, "\n") {
		// The trimmed line also matches the line shown with a different indentation.
		if !trivialSecret(line) && !structuralLine(line) {
			RegisterSecret(strings.TrimSpace(line))
		}
	}
}
