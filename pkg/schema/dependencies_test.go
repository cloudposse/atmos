package schema

import (
	"errors"
	"reflect"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUnitDependencies_UnmarshalYAML(t *testing.T) {
	input := `
commands:
  - build
  - name: test
    flags:
      env: dev
workflows:
  - build
  - name: test
    file: test.yaml
`
	var deps Dependencies
	require.NoError(t, yaml.Unmarshal([]byte(input), &deps))

	require.Len(t, deps.Commands, 2)
	assert.Equal(t, UnitDependency{Name: "build"}, deps.Commands[0])
	assert.Equal(t, "test", deps.Commands[1].Name)
	assert.Equal(t, map[string]string{"env": "dev"}, deps.Commands[1].Flags)

	require.Len(t, deps.Workflows, 2)
	assert.Equal(t, UnitDependency{Name: "build"}, deps.Workflows[0])
	assert.Equal(t, "test", deps.Workflows[1].Name)
	assert.Equal(t, "test.yaml", deps.Workflows[1].File)
}

func TestUnitDependencies_UnmarshalYAML_RejectsBadShape(t *testing.T) {
	var deps Dependencies
	err := yaml.Unmarshal([]byte("commands: not-a-list"), &deps)
	require.Error(t, err)
}

func TestUnitDependencies_UnmarshalYAML_RejectsBadElement(t *testing.T) {
	var deps Dependencies
	err := yaml.Unmarshal([]byte("commands:\n  - [nested, sequence]"), &deps)
	require.Error(t, err)
}

func TestUnitDependencyDecodeHook(t *testing.T) {
	hook := UnitDependencyDecodeHook()

	var deps Dependencies
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &deps,
		DecodeHook: hook,
	})
	require.NoError(t, err)

	input := map[string]any{
		"commands": []any{
			"build",
			map[string]any{"name": "test", "flags": map[string]any{"env": "dev"}},
		},
		"workflows": []any{
			"build",
			map[string]any{"name": "test", "file": "test.yaml"},
		},
	}
	require.NoError(t, decoder.Decode(input))

	require.Len(t, deps.Commands, 2)
	assert.Equal(t, UnitDependency{Name: "build"}, deps.Commands[0])
	assert.Equal(t, "test", deps.Commands[1].Name)
	assert.Equal(t, map[string]string{"env": "dev"}, deps.Commands[1].Flags)

	require.Len(t, deps.Workflows, 2)
	assert.Equal(t, UnitDependency{Name: "build"}, deps.Workflows[0])
	assert.Equal(t, "test.yaml", deps.Workflows[1].File)
}

func TestUnitDependencyDecodeHook_IgnoresOtherTypes(t *testing.T) {
	hook := UnitDependencyDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	// Wrong target type: pass through unchanged.
	out, err := hook(reflect.TypeOf([]any{}), reflect.TypeOf(""), []any{"build"})
	require.NoError(t, err)
	assert.Equal(t, []any{"build"}, out)

	// Wrong source kind (not a slice): pass through unchanged.
	out, err = hook(reflect.TypeOf(""), reflect.TypeOf(UnitDependencies{}), "not-a-slice")
	require.NoError(t, err)
	assert.Equal(t, "not-a-slice", out)
}

func TestUnitDependencyDecodeHook_RejectsBadElement(t *testing.T) {
	hook := UnitDependencyDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))
	_, err := hook(reflect.TypeOf([]any{}), reflect.TypeOf(UnitDependencies{}), []any{42})
	require.Error(t, err)
}

// TestUnitDependencies_UnmarshalYAML_RejectsMappingDecodeError exercises the MappingNode branch's
// own decode-error path (as opposed to RejectsBadElement, which covers the default/unexpected-kind
// branch): a mapping element with a field of the wrong shape (flags: expects a map, given a scalar)
// fails node.Decode itself.
func TestUnitDependencies_UnmarshalYAML_RejectsMappingDecodeError(t *testing.T) {
	var deps Dependencies
	err := yaml.Unmarshal([]byte("commands:\n  - name: test\n    flags: not-a-map"), &deps)
	require.Error(t, err)
}

// TestUnitDependencyDecodeHook_ConvertsTypedSlice covers sliceToAnyGeneric's reflect-based
// fallback (used when mapstructure hands the hook a concrete-typed slice, e.g. []string, rather
// than []any), which the other decode-hook tests never exercise since they all pass []any.
func TestUnitDependencyDecodeHook_ConvertsTypedSlice(t *testing.T) {
	hook := UnitDependencyDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	out, err := hook(reflect.TypeOf([]string{}), reflect.TypeOf(UnitDependencies{}), []string{"build", "test"})
	require.NoError(t, err)
	assert.Equal(t, UnitDependencies{{Name: "build"}, {Name: "test"}}, out)
}

// TestUnitDependencyDecodeHook_NonSliceDataPassesThrough covers sliceToAnyGeneric's "not a slice
// at all" branch: f.Kind() reports Slice (satisfying the hook's own type guard) but the actual
// data handed to the hook isn't slice-shaped, so it must be returned unchanged rather than erroring.
func TestUnitDependencyDecodeHook_NonSliceDataPassesThrough(t *testing.T) {
	hook := UnitDependencyDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	out, err := hook(reflect.TypeOf([]any{}), reflect.TypeOf(UnitDependencies{}), "not-actually-a-slice")
	require.NoError(t, err)
	assert.Equal(t, "not-actually-a-slice", out)
}

// TestUnitDependencyDecodeHook_RejectsMapDecodeError covers decodeUnitDependencyItem's own
// mapstructure.Decode error path (as opposed to RejectsBadElement, which covers the default/
// unsupported-item-type branch): a map item with a field of the wrong shape (flags: expects a
// map, given a scalar) fails decoder.Decode itself even with WeaklyTypedInput enabled.
func TestUnitDependencyDecodeHook_RejectsMapDecodeError(t *testing.T) {
	hook := UnitDependencyDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	_, err := hook(reflect.TypeOf([]any{}), reflect.TypeOf(UnitDependencies{}), []any{
		map[string]any{"name": "test", "flags": "not-a-map"},
	})
	require.Error(t, err)
}

// TestDependencies_OrEmpty covers both the nil-receiver default and the pass-through of an
// already-populated Dependencies value.
func TestDependencies_OrEmpty(t *testing.T) {
	t.Run("nil receiver returns zero value", func(t *testing.T) {
		var d *Dependencies
		assert.Equal(t, Dependencies{}, d.OrEmpty())
	})

	t.Run("non-nil receiver returns its value unchanged", func(t *testing.T) {
		d := &Dependencies{Commands: UnitDependencies{{Name: "build"}}}
		assert.Equal(t, *d, d.OrEmpty())
	})
}

func TestComponentDependency_IsFileDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      ComponentDependency
		expected bool
	}{
		{
			name:     "file kind returns true",
			dep:      ComponentDependency{Kind: "file", Path: "config.json"},
			expected: true,
		},
		{
			name:     "file kind without path still returns true",
			dep:      ComponentDependency{Kind: "file"},
			expected: true,
		},
		{
			name:     "folder kind returns false",
			dep:      ComponentDependency{Kind: "folder", Path: "src/"},
			expected: false,
		},
		{
			name:     "terraform kind returns false",
			dep:      ComponentDependency{Kind: "terraform", Component: "vpc"},
			expected: false,
		},
		{
			name:     "empty kind returns false",
			dep:      ComponentDependency{Component: "vpc"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dep.IsFileDependency())
		})
	}
}

func TestComponentDependency_IsFolderDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      ComponentDependency
		expected bool
	}{
		{
			name:     "folder kind returns true",
			dep:      ComponentDependency{Kind: "folder", Path: "src/lambda"},
			expected: true,
		},
		{
			name:     "folder kind without path still returns true",
			dep:      ComponentDependency{Kind: "folder"},
			expected: true,
		},
		{
			name:     "file kind returns false",
			dep:      ComponentDependency{Kind: "file", Path: "config.json"},
			expected: false,
		},
		{
			name:     "terraform kind returns false",
			dep:      ComponentDependency{Kind: "terraform", Component: "vpc"},
			expected: false,
		},
		{
			name:     "empty kind returns false",
			dep:      ComponentDependency{Component: "rds"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dep.IsFolderDependency())
		})
	}
}

func TestComponentDependency_IsComponentDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      ComponentDependency
		expected bool
	}{
		{
			name:     "terraform kind returns true",
			dep:      ComponentDependency{Kind: "terraform", Component: "vpc"},
			expected: true,
		},
		{
			name:     "helmfile kind returns true",
			dep:      ComponentDependency{Kind: "helmfile", Component: "nginx"},
			expected: true,
		},
		{
			name:     "empty kind returns true",
			dep:      ComponentDependency{Component: "vpc"},
			expected: true,
		},
		{
			name:     "packer kind returns true",
			dep:      ComponentDependency{Kind: "packer", Component: "ami"},
			expected: true,
		},
		{
			name:     "plugin kind returns true",
			dep:      ComponentDependency{Kind: "plugin", Component: "custom"},
			expected: true,
		},
		{
			name:     "file kind returns false",
			dep:      ComponentDependency{Kind: "file", Path: "config.json"},
			expected: false,
		},
		{
			name:     "folder kind returns false",
			dep:      ComponentDependency{Kind: "folder", Path: "src/"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dep.IsComponentDependency())
		})
	}
}

func TestDependencies_Normalize_NameAlias(t *testing.T) {
	t.Run("name alone is promoted to component", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{{Name: "vpc", Stack: "prod"}},
		}
		require.NoError(t, d.Normalize())
		assert.Equal(t, "vpc", d.Components[0].Component)
		assert.Empty(t, d.Components[0].Name, "Name should be cleared after promotion")
		assert.Equal(t, "prod", d.Components[0].Stack)
	})

	t.Run("component alone is left untouched", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{{Component: "vpc", Stack: "prod"}},
		}
		require.NoError(t, d.Normalize())
		assert.Equal(t, "vpc", d.Components[0].Component)
		assert.Empty(t, d.Components[0].Name)
	})

	t.Run("both name and component set to same value succeeds", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{{Name: "vpc", Component: "vpc"}},
		}
		require.NoError(t, d.Normalize())
		assert.Equal(t, "vpc", d.Components[0].Component)
		assert.Empty(t, d.Components[0].Name)
	})

	t.Run("conflicting name and component returns sentinel error", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{{Name: "subnet", Component: "vpc"}},
		}
		err := d.Normalize()
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrComponentDependencyNameConflict),
			"expected ErrComponentDependencyNameConflict, got %v", err)
	})
}

func TestDependencies_Normalize_FilesFoldersSiblings(t *testing.T) {
	t.Run("Files sibling key mirrors into Components", func(t *testing.T) {
		d := &Dependencies{
			Files: []string{"configs/lambda.json", "configs/db.json"},
		}
		require.NoError(t, d.Normalize())

		require.Len(t, d.Components, 2)
		assert.Equal(t, "file", d.Components[0].Kind)
		assert.Equal(t, "configs/lambda.json", d.Components[0].Path)
		assert.Equal(t, "file", d.Components[1].Kind)
		assert.Equal(t, "configs/db.json", d.Components[1].Path)
		// Files slice is preserved.
		assert.Equal(t, []string{"configs/lambda.json", "configs/db.json"}, d.Files)
	})

	t.Run("Folders sibling key mirrors into Components", func(t *testing.T) {
		d := &Dependencies{
			Folders: []string{"src/lambda/handler"},
		}
		require.NoError(t, d.Normalize())

		require.Len(t, d.Components, 1)
		assert.Equal(t, "folder", d.Components[0].Kind)
		assert.Equal(t, "src/lambda/handler", d.Components[0].Path)
	})

	t.Run("inline kind:file in Components backfills into Files", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{
				{Kind: "file", Path: "configs/lambda.json"},
				{Kind: "folder", Path: "src/handler"},
				{Component: "vpc"},
			},
		}
		require.NoError(t, d.Normalize())

		assert.Equal(t, []string{"configs/lambda.json"}, d.Files)
		assert.Equal(t, []string{"src/handler"}, d.Folders)
		// Components slice retains all entries (no deletion of inline file/folder).
		require.Len(t, d.Components, 3)
	})

	t.Run("v2 sibling and v1 inline produce equivalent end state", func(t *testing.T) {
		// v2 surface
		v2 := &Dependencies{
			Components: []ComponentDependency{{Name: "vpc"}},
			Files:      []string{"configs/lambda.json"},
			Folders:    []string{"src/handler"},
		}
		require.NoError(t, v2.Normalize())

		// v1 surface
		v1 := &Dependencies{
			Components: []ComponentDependency{
				{Component: "vpc"},
				{Kind: "file", Path: "configs/lambda.json"},
				{Kind: "folder", Path: "src/handler"},
			},
		}
		require.NoError(t, v1.Normalize())

		// Both produce the same Files/Folders typed views.
		assert.ElementsMatch(t, v1.Files, v2.Files)
		assert.ElementsMatch(t, v1.Folders, v2.Folders)

		// Both produce a Components slice with three entries: vpc, file, folder.
		require.Len(t, v1.Components, 3)
		require.Len(t, v2.Components, 3)
		assertHasComponentEntry(t, v1.Components, "vpc")
		assertHasComponentEntry(t, v2.Components, "vpc")
		assertHasFileEntry(t, v1.Components, "configs/lambda.json")
		assertHasFileEntry(t, v2.Components, "configs/lambda.json")
		assertHasFolderEntry(t, v1.Components, "src/handler")
		assertHasFolderEntry(t, v2.Components, "src/handler")
	})

	t.Run("dedupes paths declared via both sibling and inline", func(t *testing.T) {
		d := &Dependencies{
			Files: []string{"configs/shared.json"},
			Components: []ComponentDependency{
				{Kind: "file", Path: "configs/shared.json"},
			},
		}
		require.NoError(t, d.Normalize())

		// Files should appear exactly once in the typed view.
		assert.Equal(t, []string{"configs/shared.json"}, d.Files)
	})

	t.Run("empty path strings are skipped during sibling mirroring", func(t *testing.T) {
		d := &Dependencies{
			Files:   []string{""},
			Folders: []string{""},
		}
		require.NoError(t, d.Normalize())
		assert.Empty(t, d.Components, "empty paths should not produce Components entries")
	})

	t.Run("inline kind:file without path returns sentinel error", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{{Kind: "file"}},
		}
		err := d.Normalize()
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrComponentDependencyMissingPath),
			"expected ErrComponentDependencyMissingPath, got %v", err)
	})

	t.Run("nil receiver is a no-op", func(t *testing.T) {
		var d *Dependencies
		assert.NoError(t, d.Normalize())
	})

	t.Run("is idempotent — calling twice produces the same result", func(t *testing.T) {
		d := &Dependencies{
			Components: []ComponentDependency{{Component: "vpc"}},
			Files:      []string{"configs/lambda.json"},
			Folders:    []string{"src/handler"},
		}
		require.NoError(t, d.Normalize())
		first := append([]ComponentDependency(nil), d.Components...)
		require.NoError(t, d.Normalize())
		assert.Equal(t, first, d.Components, "second Normalize must not append duplicates")
	})
}

// Helpers for the equivalence assertion.

func assertHasComponentEntry(t *testing.T, entries []ComponentDependency, name string) {
	t.Helper()
	for i := range entries {
		if entries[i].IsComponentDependency() && entries[i].Component == name {
			return
		}
	}
	t.Errorf("expected Components to contain a component entry %q, got %#v", name, entries)
}

func assertHasFileEntry(t *testing.T, entries []ComponentDependency, path string) {
	t.Helper()
	for i := range entries {
		if entries[i].IsFileDependency() && entries[i].Path == path {
			return
		}
	}
	t.Errorf("expected Components to contain a file entry %q, got %#v", path, entries)
}

func assertHasFolderEntry(t *testing.T, entries []ComponentDependency, path string) {
	t.Helper()
	for i := range entries {
		if entries[i].IsFolderDependency() && entries[i].Path == path {
			return
		}
	}
	t.Errorf("expected Components to contain a folder entry %q, got %#v", path, entries)
}
