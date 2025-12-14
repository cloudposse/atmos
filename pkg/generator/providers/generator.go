// Package providers provides a generator for Terraform provider override files.
package providers

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Name is the unique identifier for this generator.
	Name = "providers"
	// DefaultFilenameConst is the default output filename for provider overrides.
	DefaultFilenameConst = "providers_override.tf.json"
)

// Generator generates Terraform provider override files (providers_override.tf.json).
type Generator struct{}

// Compile-time check that Generator implements generator.Generator.
var _ generator.Generator = (*Generator)(nil)

func init() {
	generator.Register(&Generator{})
}

// Name returns the unique identifier for this generator.
func (g *Generator) Name() string {
	return Name
}

// DefaultFilename returns the default output filename.
func (g *Generator) DefaultFilename() string {
	return DefaultFilenameConst
}

// ShouldGenerate returns true if the ProvidersSection has data.
func (g *Generator) ShouldGenerate(genCtx *generator.GeneratorContext) bool {
	return len(genCtx.ProvidersSection) > 0
}

// Validate checks if the generator context has required data.
func (g *Generator) Validate(genCtx *generator.GeneratorContext) error {
	defer perf.Track(nil, "generator.providers.Validate")()

	if genCtx == nil {
		return fmt.Errorf("%w: context is nil", generator.ErrInvalidContext)
	}

	// ProvidersSection can be empty (no provider overrides for component), which is valid.
	// We just won't generate anything in that case.

	return nil
}

// Generate produces the provider override content.
// The output structure wraps providers in a "provider" key as expected by Terraform.
func (g *Generator) Generate(ctx context.Context, genCtx *generator.GeneratorContext) (map[string]any, error) {
	defer perf.Track(nil, "generator.providers.Generate")()

	if err := g.Validate(genCtx); err != nil {
		return nil, err
	}

	// Return nil if no providers to generate.
	if len(genCtx.ProvidersSection) == 0 {
		return nil, nil
	}

	// Wrap providers in the expected Terraform structure.
	// The output format is:
	// {
	//   "provider": {
	//     "aws": { "region": "us-east-1", ... },
	//     "kubernetes": { ... }
	//   }
	// }
	return map[string]any{
		"provider": genCtx.ProvidersSection,
	}, nil
}
