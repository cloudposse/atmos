package loader

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Position represents a location in a source file.
type Position struct {
	// Line is the 1-based line number.
	Line int

	// Column is the 1-based column number.
	Column int
}

// Metadata contains source position information for provenance tracking.
type Metadata struct {
	// Positions maps paths (e.g., "vars.region") to their positions in the source file.
	Positions map[string]Position

	// SourceFile is the path to the source file.
	SourceFile string

	// Format is the format name (e.g., "yaml", "json", "hcl").
	Format string
}

// NewMetadata creates a new metadata struct.
func NewMetadata(sourceFile, format string) *Metadata {
	defer perf.Track(nil, "loader.NewMetadata")()

	return &Metadata{
		Positions:  make(map[string]Position),
		SourceFile: sourceFile,
		Format:     format,
	}
}

// SetPosition sets the position for a given path.
func (m *Metadata) SetPosition(path string, line, column int) {
	defer perf.Track(nil, "loader.Metadata.SetPosition")()

	if m.Positions == nil {
		m.Positions = make(map[string]Position)
	}
	m.Positions[path] = Position{Line: line, Column: column}
}

// GetPosition returns the position for a given path.
func (m *Metadata) GetPosition(path string) (Position, bool) {
	defer perf.Track(nil, "loader.Metadata.GetPosition")()

	if m == nil || m.Positions == nil {
		return Position{}, false
	}
	pos, ok := m.Positions[path]
	return pos, ok
}

// StackLoader handles loading stack configurations from a specific format.
type StackLoader interface {
	// Extensions returns supported file extensions (e.g., [".yaml", ".yml"]).
	Extensions() []string

	// Name returns a human-readable name (e.g., "YAML", "JSON", "HCL").
	Name() string

	// Load parses raw bytes into the common representation.
	// Returns map[string]any for objects, []any for arrays.
	Load(ctx context.Context, data []byte, opts ...LoadOption) (any, error)

	// LoadWithMetadata parses and returns position information for provenance.
	LoadWithMetadata(ctx context.Context, data []byte, opts ...LoadOption) (any, *Metadata, error)

	// Encode converts data back to this format (for output/round-trip).
	Encode(ctx context.Context, data any, opts ...EncodeOption) ([]byte, error)
}

// BaseLoader provides a base implementation for StackLoader that can be embedded.
type BaseLoader struct {
	LoaderName       string
	LoaderExtensions []string
}

// Name returns the loader name.
func (l *BaseLoader) Name() string {
	defer perf.Track(nil, "loader.BaseLoader.Name")()

	return l.LoaderName
}

// Extensions returns the supported file extensions.
func (l *BaseLoader) Extensions() []string {
	defer perf.Track(nil, "loader.BaseLoader.Extensions")()

	if l.LoaderExtensions == nil {
		return []string{}
	}
	return l.LoaderExtensions
}
