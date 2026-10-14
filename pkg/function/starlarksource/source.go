// Package starlarksource preserves configuration source and its original location
// through stack merges and YAML serialization, without importing an interpreter.
package starlarksource

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	Tag            = "!starlark"
	locationPrefix = "# atmos-starlark-source: "
)

// Source is an inline function body with its original YAML location.
type Source struct {
	Code string `json:"-"`
	File string `json:"file"`
	Line int32  `json:"line"`
}

// Encode keeps the body readable and carries its location in a Starlark comment.
func (s Source) Encode() string {
	defer perf.Track(nil, "starlarksource.Source.Encode")()

	metadata, _ := json.Marshal(s)
	return Tag + "\n" + locationPrefix + base64.RawURLEncoding.EncodeToString(metadata) + "\n" + s.Code
}

// Is reports whether value is a complete Starlark tag followed by its body.
func Is(value string) bool {
	defer perf.Track(nil, "starlarksource.Is")()

	return value == Tag || strings.HasPrefix(value, Tag+" ") || strings.HasPrefix(value, Tag+"\n") || strings.HasPrefix(value, Tag+"\t")
}

// IsEncoded reports whether value was produced by the YAML loader's !starlark tag.
// Unlike Is, it requires the location header written by Encode, so an ordinary string
// that merely begins with the tag text (a quoted scalar or included file) remains data.
func IsEncoded(value string) bool {
	defer perf.Track(nil, "starlarksource.IsEncoded")()

	return strings.HasPrefix(value, Tag+"\n"+locationPrefix)
}

// Decode accepts loader-preserved source or a manually supplied tagged string.
func Decode(value string) Source {
	defer perf.Track(nil, "starlarksource.Decode")()

	code := strings.TrimLeft(strings.TrimPrefix(value, Tag), " \t\r\n")
	source := Source{Code: code, File: "!starlark", Line: 1}
	if !strings.HasPrefix(code, locationPrefix) {
		return source
	}
	header, body, ok := strings.Cut(code, "\n")
	if !ok {
		return source
	}
	metadata, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(header, locationPrefix))
	if err != nil || json.Unmarshal(metadata, &source) != nil {
		return Source{Code: code, File: "!starlark", Line: 1}
	}
	source.Code = body
	return source
}
