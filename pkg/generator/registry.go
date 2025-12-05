package generator

import (
	"context"
	"fmt"
	"sort"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	registry     *GeneratorRegistry
	registryOnce sync.Once
)

// GeneratorRegistry manages generator registration and execution.
type GeneratorRegistry struct {
	mu         sync.RWMutex
	generators map[string]Generator
}

// GetRegistry returns the global generator registry singleton.
func GetRegistry() *GeneratorRegistry {
	registryOnce.Do(func() {
		registry = &GeneratorRegistry{
			generators: make(map[string]Generator),
		}
	})
	return registry
}

// Register adds a generator to the registry.
// This is typically called from a generator's init() function.
func Register(gen Generator) {
	GetRegistry().mu.Lock()
	defer GetRegistry().mu.Unlock()
	GetRegistry().generators[gen.Name()] = gen
	log.Debug("Registered generator", "name", gen.Name())
}

// Get retrieves a generator by name.
func (r *GeneratorRegistry) Get(name string) (Generator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	gen, ok := r.generators[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrGeneratorNotFound, name)
	}
	return gen, nil
}

// List returns all registered generator names in sorted order.
func (r *GeneratorRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.generators))
	for name := range r.generators {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GenerateAll runs all registered generators that should generate.
func GenerateAll(ctx context.Context, genCtx *GeneratorContext, writer Writer) error {
	defer perf.Track(nil, "generator.GenerateAll")()

	reg := GetRegistry()
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	for name, gen := range reg.generators {
		if !gen.ShouldGenerate(genCtx) {
			log.Debug("Skipping generator (no relevant config)", "generator", name)
			continue
		}

		log.Debug("Running generator", "generator", name)

		if err := gen.Validate(genCtx); err != nil {
			return fmt.Errorf("%w: generator %s: %v", ErrValidationFailed, name, err)
		}

		content, err := gen.Generate(ctx, genCtx)
		if err != nil {
			return fmt.Errorf("%w: generator %s: %v", ErrGenerationFailed, name, err)
		}

		if content == nil {
			log.Debug("Generator returned nil content, skipping write", "generator", name)
			continue
		}

		if !genCtx.DryRun {
			filename := gen.DefaultFilename()
			log.Debug("Writing generated file", "generator", name, "file", filename)
			if err := writer.WriteJSON(genCtx.WorkingDir, filename, content); err != nil {
				return fmt.Errorf("%w: generator %s: %v", ErrWriteFailed, name, err)
			}
		}
	}

	return nil
}

// Generate runs a specific generator by name.
func Generate(ctx context.Context, name string, genCtx *GeneratorContext, writer Writer) error {
	defer perf.Track(nil, "generator.Generate")()

	gen, err := GetRegistry().Get(name)
	if err != nil {
		return err
	}

	if !gen.ShouldGenerate(genCtx) {
		log.Debug("Generator has no relevant config, skipping", "generator", name)
		return nil
	}

	if err := gen.Validate(genCtx); err != nil {
		return fmt.Errorf("%w: %v", ErrValidationFailed, err)
	}

	content, err := gen.Generate(ctx, genCtx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	if content == nil {
		return nil
	}

	if !genCtx.DryRun {
		filename := gen.DefaultFilename()
		if err := writer.WriteJSON(genCtx.WorkingDir, filename, content); err != nil {
			return fmt.Errorf("%w: %v", ErrWriteFailed, err)
		}
	}

	return nil
}
