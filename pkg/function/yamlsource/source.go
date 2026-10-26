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

// prefix is process-local: authored strings cannot impersonate a loader-created
// value, even if they contain a copy of an internal representation from output.
var prefix = "!atmos-yaml-source:" + randomID() + ":"

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
	return prefix + base64.RawURLEncoding.EncodeToString(metadata) + ":" + s.Argument
}

// Decode recognizes source created by this process, never a raw function string.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Decode(value string) (Source, bool) {
	var source Source
	rest, ok := strings.CutPrefix(value, prefix)
	if !ok {
		return source, false
	}
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

// Expression returns the spelling accepted by explicit evaluation APIs.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func (s Source) Expression() string {
	if s.Argument == "" {
		return s.Tag
	}
	return s.Tag + " " + s.Argument
}

// Expression unwraps a transported value for an explicit evaluator.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Expression(value string) string {
	if source, ok := Decode(value); ok {
		return source.Expression()
	}
	return value
}

// ContainsTag reports whether text embeds source for a tag, as can happen when
// a template interpolates a computed value into a stack name.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func ContainsTag(value, tag string) bool {
	for {
		_, rest, found := strings.Cut(value, prefix)
		if !found {
			return false
		}
		if source, ok := Decode(prefix + rest); ok && source.Tag == tag {
			return true
		}
		value = rest
	}
}
