package schema

import (
	"encoding/json"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
)

// ConfigImport is one entry in the top-level CLI-config `import:` list in `atmos.yaml`
// (and in auto-discovered `.atmos.d` fragments and remote config files). Each entry accepts
// either a bare string path or an object with additional controls, bringing CLI-config imports
// toward parity with the stack-manifest import model (see StackImport).
//
// A bare string keeps the historical fail-open behavior (a failed import is warned and skipped).
// An object entry defaults to fail-closed: a required import that cannot be resolved or merged
// aborts configuration loading, unless `skip_if_missing: true` is set.
type ConfigImport struct {
	// Path is the import location: a local path/glob or a remote go-getter URL
	// (e.g. `git::https://host/org/repo.git//.atmos.d/mcp.yaml?ref=<sha>`).
	Path string `yaml:"path" json:"path" mapstructure:"path"`

	// SkipIfMissing controls failure behavior when the import cannot be resolved or merged.
	// For a bare-string entry it defaults to true (warn and skip - the historical fail-open
	// behavior). For an object entry it defaults to false (fail closed): a required import
	// that cannot be loaded aborts configuration loading. The field name and semantics mirror
	// StackImport.SkipIfMissing.
	SkipIfMissing bool `yaml:"skip_if_missing,omitempty" json:"skip_if_missing,omitempty" mapstructure:"skip_if_missing"`

	// When is an optional condition (`local`, `ci`, or a CEL expression) gating whether this
	// import is resolved at all. An unset condition always imports. It is evaluated at
	// config-load time against runtime facts (currently CI detection), letting a fragment stay
	// committed and active on workstations while auto-skipping in CI (and vice versa).
	When Condition `yaml:"when,omitempty" json:"when,omitempty" mapstructure:"when"`
}

// configImportObject is the object serialization form of ConfigImport. It carries no custom
// marshalers (avoiding infinite recursion) and uses a pointer for `when` so an unset condition is
// omitted in both YAML and JSON.
type configImportObject struct {
	Path          string     `yaml:"path" json:"path"`
	SkipIfMissing bool       `yaml:"skip_if_missing,omitempty" json:"skip_if_missing,omitempty"`
	When          *Condition `yaml:"when,omitempty" json:"when,omitempty"`
}

// isStringEquivalent reports whether the entry is equivalent to a legacy bare-string import: an
// optional import (SkipIfMissing) with no `when` condition. Such an entry serializes back to a
// plain string so `atmos describe config` output is unchanged from the string form.
func (c ConfigImport) isStringEquivalent() bool {
	return c.SkipIfMissing && c.When.IsZero()
}

func (c ConfigImport) marshalObject() configImportObject {
	obj := configImportObject{Path: c.Path, SkipIfMissing: c.SkipIfMissing}
	if !c.When.IsZero() {
		when := c.When
		obj.When = &when
	}
	return obj
}

// MarshalYAML renders a string-equivalent entry as a bare string (so existing string imports are
// unchanged in `atmos describe config` output) and any other entry as an object.
func (c ConfigImport) MarshalYAML() (any, error) {
	if c.isStringEquivalent() {
		return c.Path, nil
	}
	return c.marshalObject(), nil
}

// MarshalJSON mirrors MarshalYAML for `--format json` output.
func (c ConfigImport) MarshalJSON() ([]byte, error) {
	if c.isStringEquivalent() {
		return json.Marshal(c.Path)
	}
	return json.Marshal(c.marshalObject())
}

// ConfigImports is the top-level CLI-config `import:` list. Each entry is a string or an object;
// see ConfigImport. A mapstructure decode hook (ConfigImportsDecodeHook) normalizes both forms.
type ConfigImports []ConfigImport

// Paths returns the import paths in order. Convenience for legacy consumers that only need the
// string list and do not care about per-import controls (`skip_if_missing`, `when`).
func (c ConfigImports) Paths() []string {
	if len(c) == 0 {
		return nil
	}
	paths := make([]string, 0, len(c))
	for _, imp := range c {
		paths = append(paths, imp.Path)
	}
	return paths
}

// ConfigImportsFromPaths wraps a plain string list as ConfigImports with fail-open semantics
// (SkipIfMissing: true), matching the historical behavior of string-form imports. Used by callers
// that build an import list programmatically (e.g. provisioned-identity injection).
func ConfigImportsFromPaths(paths []string) ConfigImports {
	if len(paths) == 0 {
		return nil
	}
	imports := make(ConfigImports, 0, len(paths))
	for _, p := range paths {
		imports = append(imports, ConfigImport{Path: p, SkipIfMissing: true})
	}
	return imports
}

// ConfigImportsDecodeHook lets Viper/mapstructure decode CLI-config `import:` entries that are
// either a bare string or an object into ConfigImport. A bare string becomes
// ConfigImport{Path: <string>, SkipIfMissing: true} (fail-open, preserving historical behavior);
// an object is left for mapstructure to decode normally, so its `skip_if_missing` defaults to
// false (fail-closed) and its `when` is handled by ConditionDecodeHook.
func ConfigImportsDecodeHook() mapstructure.DecodeHookFunc {
	configImportType := reflect.TypeOf(ConfigImport{})

	return func(_ reflect.Type, to reflect.Type, data any) (any, error) {
		if to != configImportType {
			return data, nil
		}
		if s, ok := data.(string); ok {
			return ConfigImport{Path: s, SkipIfMissing: true}, nil
		}
		// Object form: let mapstructure decode the map into the struct. `when` is
		// decoded by the composed ConditionDecodeHook, and `skip_if_missing` keeps
		// its zero value (false = fail closed) when the key is absent.
		return data, nil
	}
}
