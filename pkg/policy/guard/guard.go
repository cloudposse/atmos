// Package guard supplies dependency-free pre-I/O hooks to configuration loaders.
// The policy admission adapter installs the evaluator; collectors never import CI.
package guard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Origin records execution provenance, independently of the optional display provenance.
type Origin struct {
	Provenance string `json:"provenance"`
	SourceFile string `json:"source_file"`
}

// FunctionInput identifies a YAML or template function without its secret arguments.
type FunctionInput struct {
	Name   string `json:"name"`
	Engine string `json:"engine"`
	Origin
}

// SourceInput describes the actual destination immediately before a read or download.
type SourceInput struct {
	Operation    string `json:"operation"`
	Scheme       string `json:"scheme"`
	Host         string `json:"host"`
	Path         string `json:"path"`
	Remote       bool   `json:"remote"`
	PathResolved bool   `json:"path_resolved"`
	Origin
}

const unknownProvenance = "unknown"

var state struct {
	sync.RWMutex
	evaluate func(context.Context, string, json.RawMessage) error
	enabled  func(string) (bool, error)
	origins  map[string]Origin
	values   map[[32]byte]Origin
}

// SetEvaluator connects collectors to admission without introducing dependency cycles.
func SetEvaluator(evaluate func(context.Context, string, json.RawMessage) error) {
	defer perf.Track(nil, "guard.SetEvaluator")()

	state.Lock()
	defer state.Unlock()
	state.evaluate = evaluate
}

// ResetOrigins isolates source attribution between CLI invocations.
func ResetOrigins() {
	defer perf.Track(nil, "guard.ResetOrigins")()

	state.Lock()
	defer state.Unlock()
	state.origins = make(map[string]Origin)
	state.values = make(map[[32]byte]Origin)
}

func evaluate(ctx context.Context, kind string, input any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	state.RLock()
	fn := state.evaluate
	state.RUnlock()
	if fn == nil {
		return nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return fn(ctx, kind, raw)
}

// RegisterRemote associates cached files/directories with their remote origin.
func RegisterRemote(path, source string) {
	defer perf.Track(nil, "guard.RegisterRemote")()

	absolute, err := canonicalOriginPath(path)
	if err != nil {
		return
	}
	origin := FileOrigin(source)
	state.Lock()
	defer state.Unlock()
	if state.origins == nil {
		state.origins = make(map[string]Origin)
	}
	state.origins[filepath.Clean(absolute)] = origin
}

// FileOrigin never infers that an unattributed value was authored locally.
func FileOrigin(file string) Origin {
	defer perf.Track(nil, "guard.FileOrigin")()

	if file == "" {
		return Origin{Provenance: unknownProvenance}
	}
	if isRemote(file) {
		return Origin{Provenance: "remote", SourceFile: safeSource(file)}
	}
	path, err := canonicalOriginPath(file)
	if err != nil {
		return Origin{Provenance: unknownProvenance}
	}
	state.RLock()
	defer state.RUnlock()
	best := ""
	origin := Origin{Provenance: "local", SourceFile: filepath.Clean(path)}
	for root, candidate := range state.origins {
		if within(root, path) && len(root) > len(best) {
			best = root
			origin = candidate
		}
	}
	if best == "" {
		if _, err := os.Stat(path); err != nil {
			return Origin{Provenance: unknownProvenance}
		}
	}
	return origin
}

// RememberValue conservatively retains remote attribution through merges and lazy evaluation.
// Identical values from multiple sources take the more restrictive provenance.
func RememberValue(value string, origin Origin) {
	defer perf.Track(nil, "guard.RememberValue")()

	key := sha256.Sum256([]byte(value))
	state.Lock()
	defer state.Unlock()
	if state.values == nil {
		state.values = make(map[[32]byte]Origin)
	}
	old, exists := state.values[key]
	if !exists || origin.Provenance == "remote" || old.Provenance == "local" && origin.Provenance != "local" {
		state.values[key] = origin
	}
}

// ValueOrigin retrieves attribution retained while parsing, or reports unknown.
func ValueOrigin(value string) Origin {
	defer perf.Track(nil, "guard.ValueOrigin")()

	state.RLock()
	defer state.RUnlock()
	origin, ok := state.values[sha256.Sum256([]byte(value))]
	if !ok {
		return Origin{Provenance: unknownProvenance}
	}
	return origin
}

// Function gates the actual handler invocation.
func Function(ctx context.Context, name, engine string, origin Origin) error {
	defer perf.Track(nil, "guard.Function")()

	return evaluate(ctx, "function", FunctionInput{Name: name, Engine: engine, Origin: origin})
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// SetEnabled installs the capability lookup used to preserve unconfigured behavior.
func SetEnabled(enabled func(string) (bool, error)) {
	defer perf.Track(nil, "guard.SetEnabled")()

	state.Lock()
	defer state.Unlock()
	state.enabled = enabled
}

// Enabled reports whether this invocation has rules for a capability.
func Enabled(kind string) (bool, error) {
	defer perf.Track(nil, "guard.Enabled")()

	state.RLock()
	fn := state.enabled
	state.RUnlock()
	if fn == nil {
		return false, nil
	}
	return fn(kind)
}

// RememberTree retains attribution for included scalar values before their next evaluation pass.
func RememberTree(value any, origin Origin) {
	defer perf.Track(nil, "guard.RememberTree")()

	switch node := value.(type) {
	case string:
		RememberValue(node, origin)
	case map[string]any:
		for _, child := range node {
			RememberTree(child, origin)
		}
	case map[any]any:
		for _, child := range node {
			RememberTree(child, origin)
		}
	case []any:
		for _, child := range node {
			RememberTree(child, origin)
		}
	}
}

// Resolve existing ancestors too, so nonexistent descendants retain cached-directory attribution.
func canonicalOriginPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	candidate := absolute
	suffix := ""
	for {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return absolute, nil
		}
		suffix = filepath.Join(filepath.Base(candidate), suffix)
		candidate = parent
	}
}
