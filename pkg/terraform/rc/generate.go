package rc

import (
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// labeledBlockTypes are CLI-config top-level keys whose value is a map of
// label → block body, rendered as a labeled block (e.g. host "name" { ... }).
var labeledBlockTypes = map[string]bool{
	"host":               true,
	"credentials":        true,
	"credentials_helper": true,
}

// Render converts the opaque Terraform CLI-config map (the `rc` section minus
// `enabled`) into Terraform's native CLI configuration (HCL) bytes.
//
// The CLI-config grammar is a small, fixed set of constructs that a generic
// map-walker renders incorrectly, so this is a targeted renderer:
//   - provider_installation → one block with repeated, ORDER-PRESERVING method
//     sub-blocks (network_mirror, direct, filesystem_mirror).
//   - host / credentials / credentials_helper → labeled blocks keyed by the map key.
//   - scalars and lists at the top level → top-level attributes (plugin_cache_dir,
//     disable_checkpoint, and any future scalar directive).
//   - any other map → a best-effort unlabeled block (forward-compatible passthrough).
func Render(rc map[string]any) ([]byte, error) {
	defer perf.Track(nil, "rc.Render")()

	f := hclwrite.NewEmptyFile()
	body := f.Body()

	// Iterate top-level keys in sorted order for deterministic output. The
	// provider_installation METHOD list is never sorted (order is precedence).
	for _, key := range sortedKeys(rc) {
		if err := writeTopLevel(body, key, rc[key]); err != nil {
			return nil, err
		}
	}

	return f.Bytes(), nil
}

// writeTopLevel dispatches a single top-level CLI-config key to its renderer.
func writeTopLevel(body *hclwrite.Body, key string, value any) error {
	switch {
	case key == "provider_installation":
		return writeProviderInstallation(body, value)
	case labeledBlockTypes[key]:
		return writeLabeledBlocks(body, key, value)
	}

	// Scalars and lists become top-level attributes.
	if !isMap(value) {
		ctyVal, err := toCty(value)
		if err != nil {
			return fmt.Errorf("%w: rendering CLI-config attribute %q: %w", errUtils.ErrInvalidConfig, key, err)
		}
		body.SetAttributeValue(key, ctyVal)
		return nil
	}

	// Unknown map directive: best-effort unlabeled block whose body is attributes.
	m, err := asStringMap(value)
	if err != nil {
		return fmt.Errorf("%w: rendering CLI-config block %q: %w", errUtils.ErrInvalidConfig, key, err)
	}
	block := body.AppendNewBlock(key, nil)
	return writeAttributes(block.Body(), m)
}

// writeProviderInstallation renders the single provider_installation block, whose
// body is a sequence of method sub-blocks. The value is a list of single-key maps
// (each key is a method name) per Terraform's CLI-config grammar; a bare map is
// also accepted. List order is preserved — Terraform treats method order as
// precedence — so the list is NOT sorted.
func writeProviderInstallation(body *hclwrite.Body, value any) error {
	block := body.AppendNewBlock("provider_installation", nil)

	switch v := value.(type) {
	case []any:
		for _, elem := range v {
			m, err := asStringMap(elem)
			if err != nil {
				return fmt.Errorf("%w: provider_installation entry: %w", errUtils.ErrInvalidConfig, err)
			}
			if err := writeMethods(block.Body(), m); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		return writeMethods(block.Body(), v)
	case nil:
		return nil
	default:
		return fmt.Errorf("%w: provider_installation expects a list or map, got %T", errUtils.ErrInvalidConfig, value)
	}
}

// writeMethods appends one sub-block per method in the map. Within a single map,
// keys are sorted for determinism (each list element normally holds exactly one
// method, so ordering across methods is governed by the list, not this sort).
func writeMethods(body *hclwrite.Body, methods map[string]any) error {
	for _, method := range sortedKeys(methods) {
		sub := body.AppendNewBlock(method, nil)
		// A method body may be nil (e.g. `direct: {}`) → an empty block.
		if methods[method] == nil {
			continue
		}
		m, err := asStringMap(methods[method])
		if err != nil {
			return fmt.Errorf("%w: provider_installation method %q: %w", errUtils.ErrInvalidConfig, method, err)
		}
		if err := writeAttributes(sub.Body(), m); err != nil {
			return err
		}
	}
	return nil
}

// writeLabeledBlocks renders host/credentials/credentials_helper blocks. The value
// is a map of label → block body; the label becomes the (quoted) block label.
func writeLabeledBlocks(body *hclwrite.Body, blockType string, value any) error {
	labels, err := asStringMap(value)
	if err != nil {
		return fmt.Errorf("%w: %s block: %w", errUtils.ErrInvalidConfig, blockType, err)
	}
	for _, label := range sortedKeys(labels) {
		inner, err := asStringMap(labels[label])
		if err != nil {
			return fmt.Errorf("%w: %s %q expects a map: %w", errUtils.ErrInvalidConfig, blockType, label, err)
		}
		block := body.AppendNewBlock(blockType, []string{label})
		if err := writeAttributes(block.Body(), inner); err != nil {
			return err
		}
	}
	return nil
}

// writeAttributes sets every entry of m as an HCL attribute (sorted for
// determinism). Maps render as object values (key = { ... }), lists as tuples.
func writeAttributes(body *hclwrite.Body, m map[string]any) error {
	for _, key := range sortedKeys(m) {
		ctyVal, err := toCty(m[key])
		if err != nil {
			return fmt.Errorf("%w: rendering attribute %q: %w", errUtils.ErrInvalidConfig, key, err)
		}
		body.SetAttributeValue(key, ctyVal)
	}
	return nil
}

// sortedKeys returns the keys of a map in sorted order for deterministic output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isMap reports whether value is a map decoded from YAML/JSON.
func isMap(value any) bool {
	switch value.(type) {
	case map[string]any, map[any]any:
		return true
	default:
		return false
	}
}

// asStringMap coerces a YAML/JSON-decoded value into map[string]any, tolerating
// the map[any]any shape some decoders produce; a nil value maps to an empty map.
func asStringMap(value any) (map[string]any, error) {
	switch v := value.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return v, nil
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[fmt.Sprintf("%v", k)] = val
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: expected a map, got %T", errUtils.ErrInvalidConfig, value)
	}
}

// toCty converts a YAML/JSON-decoded Go value to a cty.Value for HCL serialization.
// Tuples/objects are used (over lists/maps) to allow mixed element types; scalars
// are delegated to scalarToCty.
func toCty(value any) (cty.Value, error) {
	switch v := value.(type) {
	case []any:
		return sliceToCty(v)
	case map[string]any:
		return mapToCty(v)
	case map[any]any:
		m, err := asStringMap(v)
		if err != nil {
			return cty.NilVal, err
		}
		return mapToCty(m)
	default:
		return scalarToCty(value)
	}
}

// scalarToCty converts a scalar (or nil) YAML/JSON value to a cty.Value.
func scalarToCty(value any) (cty.Value, error) {
	switch v := value.(type) {
	case string:
		return cty.StringVal(v), nil
	case bool:
		return cty.BoolVal(v), nil
	case int:
		return cty.NumberIntVal(int64(v)), nil
	case int64:
		return cty.NumberIntVal(v), nil
	case float64:
		return cty.NumberFloatVal(v), nil
	case nil:
		return cty.NullVal(cty.DynamicPseudoType), nil
	default:
		return cty.NilVal, fmt.Errorf("%w: unsupported CLI-config value type %T", errUtils.ErrInvalidConfig, value)
	}
}

func sliceToCty(v []any) (cty.Value, error) {
	if len(v) == 0 {
		return cty.EmptyTupleVal, nil
	}
	vals := make([]cty.Value, len(v))
	for i, item := range v {
		val, err := toCty(item)
		if err != nil {
			return cty.NilVal, err
		}
		vals[i] = val
	}
	return cty.TupleVal(vals), nil
}

func mapToCty(v map[string]any) (cty.Value, error) {
	if len(v) == 0 {
		return cty.EmptyObjectVal, nil
	}
	vals := make(map[string]cty.Value, len(v))
	for key, item := range v {
		val, err := toCty(item)
		if err != nil {
			return cty.NilVal, err
		}
		vals[key] = val
	}
	return cty.ObjectVal(vals), nil
}
