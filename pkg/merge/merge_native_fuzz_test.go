package merge

import (
	"testing"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// FuzzDeepMergeNative exercises the core stack-config merge engine with arbitrary
// YAML on both sides, to catch panics from unexpected type combinations
// (map/slice/scalar mismatches, deep nesting, aliases, etc.).
func FuzzDeepMergeNative(f *testing.F) {
	f.Add("a: 1", "b: 2", false, false)
	f.Add("nested:\n  a: 1\n  b: 2", "nested:\n  b: 20\n  c: 30", false, false)
	f.Add("k: string", "k:\n  x: 1", false, false)
	f.Add("k:\n  x: 1", "k: string", false, false)
	f.Add("a: [1, 2]", "a: [3, 4]", true, false)
	f.Add("a: [1, 2]", "a: [3, 4]", false, true)
	f.Add("a: 1", "", false, false)
	f.Add("", "a: 1", false, false)

	f.Fuzz(func(t *testing.T, dstYAML, srcYAML string, appendSlice, sliceDeepCopy bool) {
		dst, err := u.UnmarshalYAML[map[string]any](dstYAML)
		if err != nil {
			return
		}
		src, err := u.UnmarshalYAML[map[string]any](srcYAML)
		if err != nil {
			return
		}
		if dst == nil {
			dst = map[string]any{}
		}
		_ = deepMergeNative(dst, src, appendSlice, sliceDeepCopy)
	})
}
