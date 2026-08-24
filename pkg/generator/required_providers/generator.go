// Package required_providers provides a generator for Terraform required_providers blocks.
// This implements DEV-3124: Pin provider versions of components using terraform required_providers.
package required_providers

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Name is the unique identifier for this generator.
	Name = "required_providers"
	// DefaultFilenameConst is the default output filename for required_providers blocks.
	DefaultFilenameConst = "terraform_override.tf.json"
)

// Generator generates Terraform required_providers blocks (terraform_override.tf.json).
// This enables users to pin provider versions via stack configuration:
//
//	terraform:
//	  required_version: ">= 1.10.1"
//	  required_providers:
//	    aws:
//	      source: "hashicorp/aws"
//	      version: "~> 5.0"
type Generator struct{}

// Compile-time check that Generator implements generator.Generator.
var _ generator.Generator = (*Generator)(nil)

func init() {
	generator.Register(&Generator{})
}

// Name returns the unique identifier for this generator.
func (g *Generator) Name() string {
	defer perf.Track(nil, "generator.required_providers.Name")()

	return Name
}

// DefaultFilename returns the default output filename.
func (g *Generator) DefaultFilename() string {
	defer perf.Track(nil, "generator.required_providers.DefaultFilename")()

	return DefaultFilenameConst
}

// ShouldGenerate returns true if RequiredVersion or RequiredProviders has data.
func (g *Generator) ShouldGenerate(genCtx *generator.GeneratorContext) bool {
	defer perf.Track(nil, "generator.required_providers.ShouldGenerate")()

	if genCtx == nil {
		return false
	}
	return genCtx.RequiredVersion != "" || len(genCtx.RequiredProviders) > 0
}

// Validate checks if the generator context has valid data.
func (g *Generator) Validate(genCtx *generator.GeneratorContext) error {
	defer perf.Track(nil, "generator.required_providers.Validate")()

	if genCtx == nil {
		return fmt.Errorf("%w: context is nil", generator.ErrInvalidContext)
	}

	// Validate that each required_provider has a source field.
	// The source field is required by Terraform.
	for name, config := range genCtx.RequiredProviders {
		if _, ok := config["source"]; !ok {
			return fmt.Errorf("%w: provider '%s' missing 'source' field",
				generator.ErrMissingProviderSource, name)
		}
	}

	return nil
}

// Generate produces the terraform block with required_version and required_providers.
// The output structure is:
//
//	{
//	  "terraform": {
//	    "required_version": ">= 1.10.1",
//	    "required_providers": {
//	      "aws": {
//	        "source": "hashicorp/aws",
//	        "version": "~> 5.0"
//	      }
//	    }
//	  }
//	}
func (g *Generator) Generate(ctx context.Context, genCtx *generator.GeneratorContext) (map[string]any, error) {
	defer perf.Track(nil, "generator.required_providers.Generate")()

	if err := g.Validate(genCtx); err != nil {
		return nil, err
	}

	// Return nil if nothing to generate.
	if genCtx.RequiredVersion == "" && len(genCtx.RequiredProviders) == 0 {
		return nil, nil
	}

	terraformBlock := make(map[string]any)

	if genCtx.RequiredVersion != "" {
		terraformBlock["required_version"] = genCtx.RequiredVersion
	}

	if len(genCtx.RequiredProviders) > 0 {
		// Convert map[string]map[string]any to map[string]any for JSON serialization.
		providers := make(map[string]any)
		for name, config := range genCtx.RequiredProviders {
			providers[name] = config
		}
		terraformBlock["required_providers"] = providers
	}

	return map[string]any{
		"terraform": terraformBlock,
	}, nil
}
