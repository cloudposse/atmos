// Package yamlsource preserves explicit YAML function identity across the
// string-valued fields and serialization boundaries in the configuration pipeline.
// It has no dependency on configuration or evaluators.
package yamlsource

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// BehaviorKey identifies the edition transition for automatic YAML function evaluation.
const BehaviorKey = "yaml_functions.explicit_tags"

// processID makes prefix and terminator process-local: authored strings cannot
// impersonate a loader-created value, even if they contain a copy of an internal
// representation from output. The terminator marks where a source ends, so text
// a template places after a source is never read as part of its argument.
var (
	processID  = randomID()
	prefix     = "!atmos-yaml-source:" + processID + ":"
	terminator = ":atmos-yaml-source-end:" + processID
)

func randomID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(id[:])
}

// Source identifies an explicit tag and its original scalar argument.
type Source struct {
	Tag      string `json:"tag"`
	Argument string `json:"-"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

// Encode transports source through existing string fields. The argument stays
// readable so template rendering can operate on it without losing tag identity.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func (s Source) Encode() string {
	metadata, _ := json.Marshal(s)
	return prefix + base64.RawURLEncoding.EncodeToString(metadata) + ":" + s.Argument + terminator
}

// Decode recognizes source created by this process, never a raw function string.
// The value must be exactly one source; see Embedded for sources inside larger text.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Decode(value string) (Source, bool) {
	start, end, ok := locate(value)
	if !ok || start != 0 || end != len(value) {
		return Source{}, false
	}
	return parse(value)
}

// locate finds the first complete source in value, moving past any prefix that is
// never closed, such as one a template function truncated.
func locate(value string) (start, end int, ok bool) {
	for offset := 0; ; {
		index := strings.Index(value[offset:], prefix)
		if index < 0 {
			return 0, 0, false
		}
		start = offset + index
		if end, ok = sourceEnd(value, start); ok {
			return start, end, true
		}
		offset = start + len(prefix)
	}
}

// sourceEnd returns where the source beginning at start ends. Arguments can contain
// other sources after template rendering, so sources are matched like nested brackets.
func sourceEnd(value string, start int) (int, bool) {
	position := start + len(prefix)
	for depth := 1; depth > 0; {
		closing := strings.Index(value[position:], terminator)
		if closing < 0 {
			return 0, false
		}
		opening := strings.Index(value[position:], prefix)
		if opening >= 0 && opening < closing {
			depth++
			position += opening + len(prefix)
			continue
		}
		depth--
		position += closing + len(terminator)
	}
	return position, true
}

// parse reads one complete source, including its metadata.
func parse(value string) (Source, bool) {
	var source Source
	rest := strings.TrimSuffix(strings.TrimPrefix(value, prefix), terminator)
	metadata, argument, ok := strings.Cut(rest, ":")
	if !ok {
		return source, false
	}
	data, err := base64.RawURLEncoding.DecodeString(metadata)
	if err != nil || json.Unmarshal(data, &source) != nil || !strings.HasPrefix(source.Tag, "!") {
		return Source{}, false
	}
	source.Argument = argument
	return source, true
}

// Replace substitutes each top-level source in value with the result of resolve,
// leaving surrounding text and malformed sources untouched. The callback receives
// the source's transport string and its decoded form.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Replace(value string, resolve func(token string, source Source) (string, error)) (string, error) {
	var output strings.Builder
	for {
		start, end, ok := locate(value)
		if !ok {
			output.WriteString(value)
			return output.String(), nil
		}
		token := value[start:end]
		source, valid := parse(token)
		output.WriteString(value[:start])
		if valid {
			replacement, err := resolve(token, source)
			if err != nil {
				return "", err
			}
			output.WriteString(replacement)
		} else {
			output.WriteString(token)
		}
		value = value[end:]
	}
}

// Embedded reports whether value holds source that isn't the entire value: either
// text a template interpolated around it, or source a template function rewrote.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Embedded(value string) bool {
	if _, whole := Decode(value); whole {
		return false
	}
	return Contains(value) || Transformed(value)
}

// Transformed reports whether a template function rewrote source from this process,
// for example by changing its case. Its argument can no longer be recovered.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Transformed(value string) bool {
	if strings.Contains(value, prefix) {
		return false
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(prefix))
}

// Expression returns the spelling accepted by explicit evaluation APIs.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func (s Source) Expression() string {
	if s.Argument == "" {
		return s.Tag
	}
	return s.Tag + " " + s.Argument
}

// Expression unwraps a transported value for an explicit evaluator. Source inside
// larger text is shown the same way, so internal metadata never reaches output.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Expression(value string) string {
	if source, ok := Decode(value); ok {
		return source.Expression()
	}
	// The resolver below never fails.
	readable, _ := Replace(value, func(_ string, source Source) (string, error) {
		return source.Expression(), nil
	})
	return readable
}

// Comparable returns value with each source reduced to its tag and argument, so two
// values can be compared regardless of metadata such as checkout paths. The delimiters
// are process-local like the transport itself, so literal text can't spell a source.
// Compare results only within one process.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Comparable(value string) string {
	// The resolver below never fails.
	normalized, _ := Replace(value, func(_ string, source Source) (string, error) {
		return prefix + source.Tag + ":" + Comparable(source.Argument) + terminator, nil
	})
	return normalized
}

// ContainsTag reports whether text embeds source for a tag, as can happen when
// a template interpolates a computed value into a stack name.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func ContainsTag(value, tag string) bool {
	found := false
	// The resolver below never fails.
	_, _ = Replace(value, func(token string, source Source) (string, error) {
		found = found || source.Tag == tag || ContainsTag(source.Argument, tag)
		return token, nil
	})
	return found
}

// Contains reports whether text includes loader-preserved source, including
// expressions interpolated into a larger string without evaluation.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Contains(value string) bool {
	found := false
	// The resolver below never fails.
	_, _ = Replace(value, func(token string, _ Source) (string, error) {
		found = true
		return token, nil
	})
	return found
}
