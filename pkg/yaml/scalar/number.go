package scalar

import (
	"encoding/json"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	decimalBase = 10
	numberBits  = 64
)

// Number retains a JSON decimal that a native integer or float cannot round-trip.
// Its explicit standard YAML tag preserves that decimal across stack template rendering.
type Number string

func (n Number) MarshalJSON() ([]byte, error) {
	defer perf.Track(nil, "scalar.Number.MarshalJSON")()
	return json.Marshal(json.Number(n))
}

func (n Number) yamlTag() string {
	if strings.ContainsAny(string(n), ".eE") {
		return floatTag
	}
	return "!!int"
}

func (n Number) MarshalYAML() (any, error) {
	defer perf.Track(nil, "scalar.Number.MarshalYAML")()
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: n.yamlTag(), Style: yaml.TaggedStyle, Value: string(n)}, nil
}

// NativeNumber keeps ordinary native types where re-encoding them yields the same number.
func NativeNumber(n json.Number) any {
	defer perf.Track(nil, "scalar.NativeNumber")()
	if i, err := n.Int64(); err == nil {
		return i
	}
	if u, err := strconv.ParseUint(n.String(), decimalBase, numberBits); err == nil {
		return u
	}
	if f, err := n.Float64(); err == nil && strconv.FormatFloat(f, 'g', -1, numberBits) == n.String() {
		return f
	}
	return Number(n)
}
