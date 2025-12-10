package loader

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockLoader is a test implementation of StackLoader.
type mockLoader struct {
	BaseLoader
}

func (l *mockLoader) Load(ctx context.Context, data []byte, opts ...LoadOption) (any, error) {
	return nil, nil
}

func (l *mockLoader) LoadWithMetadata(ctx context.Context, data []byte, opts ...LoadOption) (any, *Metadata, error) {
	return nil, nil, nil
}

func (l *mockLoader) Encode(ctx context.Context, data any, opts ...EncodeOption) ([]byte, error) {
	return nil, nil
}

func newMockLoader(name string, extensions []string) *mockLoader {
	return &mockLoader{
		BaseLoader: BaseLoader{
			LoaderName:       name,
			LoaderExtensions: extensions,
		},
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	assert.NotNil(t, r)
	assert.Equal(t, 0, r.Len())
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml", ".yml"})
	err := r.Register(loader)

	require.NoError(t, err)
	assert.Equal(t, 1, r.Len())
	assert.True(t, r.HasExtension(".yaml"))
	assert.True(t, r.HasExtension(".yml"))
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()

	loader1 := newMockLoader("YAML", []string{".yaml"})
	loader2 := newMockLoader("YAML", []string{".yml"})

	err := r.Register(loader1)
	require.NoError(t, err)

	err = r.Register(loader2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDuplicateLoader))
}

func TestRegistryRegisterExtensionConflict(t *testing.T) {
	r := NewRegistry()

	loader1 := newMockLoader("YAML", []string{".yaml"})
	loader2 := newMockLoader("AnotherYAML", []string{".yaml"})

	err := r.Register(loader1)
	require.NoError(t, err)

	err = r.Register(loader2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDuplicateLoader))
}

func TestRegistryGetByExtension(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml", ".yml"})
	require.NoError(t, r.Register(loader))

	got, err := r.GetByExtension(".yaml")
	require.NoError(t, err)
	assert.Equal(t, loader, got)

	got, err = r.GetByExtension(".yml")
	require.NoError(t, err)
	assert.Equal(t, loader, got)
}

func TestRegistryGetByExtensionWithoutDot(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml"})
	require.NoError(t, r.Register(loader))

	// Should work without leading dot.
	got, err := r.GetByExtension("yaml")
	require.NoError(t, err)
	assert.Equal(t, loader, got)
}

func TestRegistryGetByExtensionCaseInsensitive(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml"})
	require.NoError(t, r.Register(loader))

	got, err := r.GetByExtension(".YAML")
	require.NoError(t, err)
	assert.Equal(t, loader, got)

	got, err = r.GetByExtension(".Yaml")
	require.NoError(t, err)
	assert.Equal(t, loader, got)
}

func TestRegistryGetByExtensionNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.GetByExtension(".unknown")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLoaderNotFound))
}

func TestRegistryGetByName(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml"})
	require.NoError(t, r.Register(loader))

	got, err := r.GetByName("YAML")
	require.NoError(t, err)
	assert.Equal(t, loader, got)
}

func TestRegistryGetByNameNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.GetByName("NonExistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLoaderNotFound))
}

func TestRegistryHasExtension(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml", ".yml"})
	require.NoError(t, r.Register(loader))

	assert.True(t, r.HasExtension(".yaml"))
	assert.True(t, r.HasExtension(".yml"))
	assert.True(t, r.HasExtension("yaml")) // Without dot.
	assert.True(t, r.HasExtension("YML"))  // Case insensitive.
	assert.False(t, r.HasExtension(".json"))
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()

	loader1 := newMockLoader("YAML", []string{".yaml"})
	loader2 := newMockLoader("JSON", []string{".json"})
	loader3 := newMockLoader("HCL", []string{".hcl"})

	require.NoError(t, r.Register(loader1))
	require.NoError(t, r.Register(loader2))
	require.NoError(t, r.Register(loader3))

	names := r.List()
	sort.Strings(names)

	assert.Equal(t, []string{"HCL", "JSON", "YAML"}, names)
}

func TestRegistryExtensions(t *testing.T) {
	r := NewRegistry()

	loader1 := newMockLoader("YAML", []string{".yaml", ".yml"})
	loader2 := newMockLoader("JSON", []string{".json"})

	require.NoError(t, r.Register(loader1))
	require.NoError(t, r.Register(loader2))

	exts := r.Extensions()
	sort.Strings(exts)

	assert.Equal(t, []string{".json", ".yaml", ".yml"}, exts)
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()

	loader := newMockLoader("YAML", []string{".yaml", ".yml"})
	require.NoError(t, r.Register(loader))

	err := r.Unregister("YAML")
	require.NoError(t, err)

	assert.False(t, r.HasExtension(".yaml"))
	assert.False(t, r.HasExtension(".yml"))
	assert.Equal(t, 0, r.Len())
}

func TestRegistryUnregisterNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Unregister("NonExistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLoaderNotFound))
}

func TestRegistryClear(t *testing.T) {
	r := NewRegistry()

	loader1 := newMockLoader("YAML", []string{".yaml"})
	loader2 := newMockLoader("JSON", []string{".json"})
	require.NoError(t, r.Register(loader1))
	require.NoError(t, r.Register(loader2))

	assert.Equal(t, 2, r.Len())

	r.Clear()

	assert.Equal(t, 0, r.Len())
	assert.False(t, r.HasExtension(".yaml"))
	assert.False(t, r.HasExtension(".json"))
}

// TestRegistryConcurrentAccess tests thread safety of the registry.
// NOTE: For proper race condition detection, run this test with the -race flag:
//
//	go test -race ./pkg/stack/loader/...
//
// TODO: Consider adding -race to the CI test targets (make testacc) to catch
// race conditions automatically. This would require updating the Makefile.
func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool)

	// Concurrent registration.
	go func() {
		for i := 0; i < 100; i++ {
			loader := newMockLoader("YAML", []string{".yaml"})
			_ = r.Register(loader)
			_ = r.Unregister("YAML")
		}
		done <- true
	}()

	// Concurrent reads.
	go func() {
		for i := 0; i < 100; i++ {
			_ = r.HasExtension(".yaml")
			_, _ = r.GetByExtension(".yaml")
			_ = r.List()
		}
		done <- true
	}()

	<-done
	<-done
}

func TestNormalizeExtension(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{".yaml", ".yaml"},
		{"yaml", ".yaml"},
		{".YAML", ".yaml"},
		{"YAML", ".yaml"},
		{".Yaml", ".yaml"},
		{"  .yaml  ", ".yaml"},
		{"  yaml  ", ".yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeExtension(tt.input))
		})
	}
}

func TestMetadata(t *testing.T) {
	m := NewMetadata("config.yaml", "yaml")

	assert.Equal(t, "config.yaml", m.SourceFile)
	assert.Equal(t, "yaml", m.Format)
	assert.NotNil(t, m.Positions)

	m.SetPosition("vars.region", 10, 5)
	m.SetPosition("vars.env", 11, 5)

	pos, ok := m.GetPosition("vars.region")
	assert.True(t, ok)
	assert.Equal(t, 10, pos.Line)
	assert.Equal(t, 5, pos.Column)

	pos, ok = m.GetPosition("vars.env")
	assert.True(t, ok)
	assert.Equal(t, 11, pos.Line)
	assert.Equal(t, 5, pos.Column)

	_, ok = m.GetPosition("nonexistent")
	assert.False(t, ok)
}

func TestMetadataNil(t *testing.T) {
	var m *Metadata

	_, ok := m.GetPosition("any")
	assert.False(t, ok)
}

func TestLoadOptions(t *testing.T) {
	opts := ApplyLoadOptions(
		WithStrictMode(true),
		WithAllowDuplicateKeys(false),
		WithPreserveComments(true),
		WithSourceFile("test.yaml"),
	)

	assert.True(t, opts.StrictMode)
	assert.False(t, opts.AllowDuplicateKeys)
	assert.True(t, opts.PreserveComments)
	assert.Equal(t, "test.yaml", opts.SourceFile)
}

func TestLoadOptionsDefaults(t *testing.T) {
	opts := DefaultLoadOptions()

	assert.False(t, opts.StrictMode)
	assert.True(t, opts.AllowDuplicateKeys)
	assert.False(t, opts.PreserveComments)
	assert.Equal(t, "", opts.SourceFile)
}

func TestEncodeOptions(t *testing.T) {
	opts := ApplyEncodeOptions(
		WithIndent("    "),
		WithSortKeys(true),
		WithIncludeComments(true),
		WithCompactOutput(true),
	)

	assert.Equal(t, "    ", opts.Indent)
	assert.True(t, opts.SortKeys)
	assert.True(t, opts.IncludeComments)
	assert.True(t, opts.CompactOutput)
}

func TestEncodeOptionsDefaults(t *testing.T) {
	opts := DefaultEncodeOptions()

	assert.Equal(t, "  ", opts.Indent)
	assert.False(t, opts.SortKeys)
	assert.False(t, opts.IncludeComments)
	assert.False(t, opts.CompactOutput)
}

func TestBaseLoader(t *testing.T) {
	bl := &BaseLoader{
		LoaderName:       "TestLoader",
		LoaderExtensions: []string{".test", ".tst"},
	}

	assert.Equal(t, "TestLoader", bl.Name())
	assert.Equal(t, []string{".test", ".tst"}, bl.Extensions())
}

func TestBaseLoaderNilExtensions(t *testing.T) {
	bl := &BaseLoader{
		LoaderName: "TestLoader",
	}

	exts := bl.Extensions()
	assert.NotNil(t, exts)
	assert.Len(t, exts, 0)
}
