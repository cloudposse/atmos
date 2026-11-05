package schema

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"gopkg.in/yaml.v3"
)

// SkillVersionMarker preserves a deferred YAML tag across scalar configuration merges.
var errSkillRefScalar = errors.New("skill ref must be a scalar")

const SkillVersionMarker = "\x00atmos-skill-version:"

// SkillRef distinguishes a literal Git ref from a deferred version dependency.
type SkillRef struct {
	Literal    string `json:"literal,omitempty" mapstructure:"literal"`
	Dependency string `json:"dependency,omitempty" mapstructure:"dependency"`
}

// IsZero reports whether no ref was supplied.
func (r SkillRef) IsZero() bool { return r.Literal == "" && r.Dependency == "" }

// MarshalYAML preserves the public scalar representation.
func (r SkillRef) MarshalYAML() (any, error) {
	if r.Dependency != "" {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!version", Value: r.Dependency}, nil
	}
	return r.Literal, nil
}

// UnmarshalYAML retains version references without resolving them.
func (r *SkillRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return errSkillRefScalar
	}
	*r = SkillRef{Literal: n.Value}
	if n.Tag == "!version" {
		*r = SkillRef{Dependency: n.Value}
	}
	return nil
}

// SkillRefDecodeHook restores deferred refs after configuration merging.
func SkillRefDecodeHook() mapstructure.DecodeHookFuncType {
	return func(from, to reflect.Type, value any) (any, error) {
		if to != reflect.TypeOf(SkillRef{}) || from.Kind() != reflect.String {
			return value, nil
		}
		raw := value.(string)
		if dep, ok := strings.CutPrefix(raw, SkillVersionMarker); ok {
			return SkillRef{Dependency: dep}, nil
		}
		return SkillRef{Literal: raw}, nil
	}
}

// MarshalJSON represents refs as scalar expressions in exported configuration.
func (r SkillRef) MarshalJSON() ([]byte, error) {
	value := r.Literal
	if r.Dependency != "" {
		value = "!version " + r.Dependency
	}
	return json.Marshal(value)
}
