package generator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// testGenerator is a mock generator for testing.
type testGenerator struct {
	name           string
	filename       string
	shouldGenerate bool
	validateErr    error
	generateErr    error
	content        map[string]any
}

func (g *testGenerator) Name() string {
	return g.name
}

func (g *testGenerator) DefaultFilename() string {
	return g.filename
}

func (g *testGenerator) ShouldGenerate(_ *GeneratorContext) bool {
	return g.shouldGenerate
}

func (g *testGenerator) Validate(_ *GeneratorContext) error {
	return g.validateErr
}

func (g *testGenerator) Generate(_ context.Context, _ *GeneratorContext) (map[string]any, error) {
	if g.generateErr != nil {
		return nil, g.generateErr
	}
	return g.content, nil
}

func TestGeneratorContext(t *testing.T) {
	t.Run("NewGeneratorContext populates fields from StackInfo", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg:          "vpc",
			Stack:                     "dev-us-east-1",
			ComponentFolderPrefix:     "/path/to/component",
			ComponentVarsSection:      map[string]any{"vpc_cidr": "10.0.0.0/16"},
			ComponentProvidersSection: map[string]any{"aws": map[string]any{"region": "us-east-1"}},
			ComponentBackendType:      "s3",
			ComponentBackendSection:   map[string]any{"bucket": "tf-state"},
			DryRun:                    true,
		}

		ctx := NewGeneratorContext(atmosConfig, info, "/working/dir")

		assert.Equal(t, atmosConfig, ctx.AtmosConfig)
		assert.Equal(t, info, ctx.StackInfo)
		assert.Equal(t, "vpc", ctx.Component)
		assert.Equal(t, "dev-us-east-1", ctx.Stack)
		assert.Equal(t, "/path/to/component", ctx.ComponentPath)
		assert.Equal(t, "/working/dir", ctx.WorkingDir)
		assert.Equal(t, map[string]any{"vpc_cidr": "10.0.0.0/16"}, ctx.VarsSection)
		assert.Equal(t, map[string]any{"aws": map[string]any{"region": "us-east-1"}}, ctx.ProvidersSection)
		assert.Equal(t, "s3", ctx.BackendType)
		assert.Equal(t, map[string]any{"bucket": "tf-state"}, ctx.BackendConfig)
		assert.True(t, ctx.DryRun)
		assert.Equal(t, FormatJSON, ctx.Format)
	})

	t.Run("NewGeneratorContextWithOptions applies options", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev",
		}

		ctx := NewGeneratorContextWithOptions(
			atmosConfig,
			info,
			"/working/dir",
			WithFormat(FormatHCL),
			WithDryRun(true),
		)

		assert.Equal(t, FormatHCL, ctx.Format)
		assert.True(t, ctx.DryRun)
	})
}

func TestMockWriter(t *testing.T) {
	t.Run("WriteJSON captures content", func(t *testing.T) {
		writer := NewMockWriter()
		data := map[string]any{"key": "value"}

		err := writer.WriteJSON("/tmp", "test.json", data)

		require.NoError(t, err)
		written, ok := writer.GetWritten("/tmp", "test.json")
		require.True(t, ok)
		assert.Equal(t, data, written)
	})

	t.Run("WriteHCL captures content", func(t *testing.T) {
		writer := NewMockWriter()
		data := map[string]any{"key": "value"}

		err := writer.WriteHCL("/tmp", "test.tf", data)

		require.NoError(t, err)
		written, ok := writer.GetWritten("/tmp", "test.tf")
		require.True(t, ok)
		assert.Equal(t, data, written)
	})

	t.Run("WriteErr is returned", func(t *testing.T) {
		writer := NewMockWriter()
		writer.WriteErr = errors.New("write failed")

		err := writer.WriteJSON("/tmp", "test.json", map[string]any{})

		assert.Error(t, err)
		assert.Equal(t, "write failed", err.Error())
	})

	t.Run("Clear resets state", func(t *testing.T) {
		writer := NewMockWriter()
		writer.WriteErr = errors.New("error")
		_ = writer.WriteJSON("/tmp", "test.json", map[string]any{"key": "value"})

		writer.Clear()

		assert.Nil(t, writer.WriteErr)
		assert.Empty(t, writer.Written)
	})
}

func TestGeneratorRegistry(t *testing.T) {
	// Save and restore registry state.
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()

	// Create a fresh registry for this test.
	registry = &GeneratorRegistry{
		generators: make(map[string]Generator),
	}

	t.Run("Register and Get generator", func(t *testing.T) {
		gen := &testGenerator{name: "test-gen", filename: "test.tf.json"}
		Register(gen)

		retrieved, err := GetRegistry().Get("test-gen")

		require.NoError(t, err)
		assert.Equal(t, "test-gen", retrieved.Name())
	})

	t.Run("Get returns error for unknown generator", func(t *testing.T) {
		_, err := GetRegistry().Get("unknown")

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrGeneratorNotFound))
	})

	t.Run("List returns sorted names", func(t *testing.T) {
		registry.generators = make(map[string]Generator)
		Register(&testGenerator{name: "zebra"})
		Register(&testGenerator{name: "alpha"})
		Register(&testGenerator{name: "beta"})

		names := GetRegistry().List()

		assert.Equal(t, []string{"alpha", "beta", "zebra"}, names)
	})
}

func TestGenerateAll(t *testing.T) {
	// Save and restore registry state.
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()

	t.Run("runs all generators that should generate", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"gen1": &testGenerator{
					name:           "gen1",
					filename:       "gen1.tf.json",
					shouldGenerate: true,
					content:        map[string]any{"gen": "1"},
				},
				"gen2": &testGenerator{
					name:           "gen2",
					filename:       "gen2.tf.json",
					shouldGenerate: true,
					content:        map[string]any{"gen": "2"},
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := GenerateAll(context.Background(), ctx, writer)

		require.NoError(t, err)
		assert.Len(t, writer.Written, 2)
	})

	t.Run("skips generators that should not generate", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"active": &testGenerator{
					name:           "active",
					filename:       "active.tf.json",
					shouldGenerate: true,
					content:        map[string]any{"active": true},
				},
				"inactive": &testGenerator{
					name:           "inactive",
					filename:       "inactive.tf.json",
					shouldGenerate: false,
					content:        map[string]any{"inactive": true},
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := GenerateAll(context.Background(), ctx, writer)

		require.NoError(t, err)
		assert.Len(t, writer.Written, 1)
		_, hasActive := writer.GetWritten("/tmp", "active.tf.json")
		assert.True(t, hasActive)
		_, hasInactive := writer.GetWritten("/tmp", "inactive.tf.json")
		assert.False(t, hasInactive)
	})

	t.Run("returns validation error", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"invalid": &testGenerator{
					name:           "invalid",
					shouldGenerate: true,
					validateErr:    errors.New("validation failed"),
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := GenerateAll(context.Background(), ctx, writer)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrValidationFailed))
	})

	t.Run("returns generation error", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"failing": &testGenerator{
					name:           "failing",
					shouldGenerate: true,
					generateErr:    errors.New("generation failed"),
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := GenerateAll(context.Background(), ctx, writer)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrGenerationFailed))
	})

	t.Run("respects DryRun mode", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"gen": &testGenerator{
					name:           "gen",
					filename:       "gen.tf.json",
					shouldGenerate: true,
					content:        map[string]any{"key": "value"},
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp", DryRun: true}
		writer := NewMockWriter()

		err := GenerateAll(context.Background(), ctx, writer)

		require.NoError(t, err)
		assert.Empty(t, writer.Written, "DryRun should not write files")
	})

	t.Run("skips nil content", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"nil-gen": &testGenerator{
					name:           "nil-gen",
					filename:       "nil.tf.json",
					shouldGenerate: true,
					content:        nil,
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := GenerateAll(context.Background(), ctx, writer)

		require.NoError(t, err)
		assert.Empty(t, writer.Written)
	})
}

func TestGenerate(t *testing.T) {
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()

	t.Run("runs single generator by name", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: map[string]Generator{
				"single": &testGenerator{
					name:           "single",
					filename:       "single.tf.json",
					shouldGenerate: true,
					content:        map[string]any{"single": true},
				},
			},
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := Generate(context.Background(), "single", ctx, writer)

		require.NoError(t, err)
		written, ok := writer.GetWritten("/tmp", "single.tf.json")
		require.True(t, ok)
		assert.Equal(t, map[string]any{"single": true}, written)
	})

	t.Run("returns error for unknown generator", func(t *testing.T) {
		registry = &GeneratorRegistry{
			generators: make(map[string]Generator),
		}

		ctx := &GeneratorContext{WorkingDir: "/tmp"}
		writer := NewMockWriter()

		err := Generate(context.Background(), "unknown", ctx, writer)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrGeneratorNotFound))
	})
}

func TestOptions(t *testing.T) {
	t.Run("WithFormat sets format", func(t *testing.T) {
		ctx := &GeneratorContext{}
		ApplyOptions(ctx, WithFormat(FormatHCL))
		assert.Equal(t, FormatHCL, ctx.Format)
	})

	t.Run("WithDryRun sets dry run", func(t *testing.T) {
		ctx := &GeneratorContext{}
		ApplyOptions(ctx, WithDryRun(true))
		assert.True(t, ctx.DryRun)
	})

	t.Run("WithWorkingDir sets working dir", func(t *testing.T) {
		ctx := &GeneratorContext{}
		ApplyOptions(ctx, WithWorkingDir("/new/dir"))
		assert.Equal(t, "/new/dir", ctx.WorkingDir)
	})
}

func TestFileWriter(t *testing.T) {
	t.Run("NewFileWriter with options", func(t *testing.T) {
		writer := NewFileWriter(WithFileMode(0o644))
		assert.Equal(t, 0o644, int(writer.fileMode))
	})

	t.Run("NewFileWriter defaults to 0o600", func(t *testing.T) {
		writer := NewFileWriter()
		assert.Equal(t, 0o600, int(writer.fileMode))
	})

	t.Run("WriteJSON creates correct path", func(t *testing.T) {
		// This is a smoke test - actual file writing is tested in integration tests.
		tmpDir := t.TempDir()
		writer := NewFileWriter()

		data := map[string]any{"test": "data"}
		err := writer.WriteJSON(tmpDir, "test.json", data)

		require.NoError(t, err)
		// Verify file exists.
		assert.FileExists(t, filepath.Join(tmpDir, "test.json"))
	})
}
