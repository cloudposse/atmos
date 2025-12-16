// Package varfile provides a generator for Terraform variable files (.tfvars.json).
package varfile

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Name is the unique identifier for this generator.
	Name = "varfile"
)

// Generator generates Terraform variable files (.tfvars.json).
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
// Note: For varfiles, the actual filename is dynamically constructed based on
// component and stack context. This returns a placeholder that should be
// overridden by the calling code.
func (g *Generator) DefaultFilename() string {
	// Varfile names are dynamic: {context}-{component}.terraform.tfvars.json
	// The actual filename is constructed in the calling code based on StackInfo.
	return "terraform.tfvars.json"
}

// ShouldGenerate returns true if the VarsSection has data.
func (g *Generator) ShouldGenerate(genCtx *generator.GeneratorContext) bool {
	if genCtx == nil {
		return false
	}
	return len(genCtx.VarsSection) > 0
}

// Validate checks if the generator context has required data.
func (g *Generator) Validate(genCtx *generator.GeneratorContext) error {
	defer perf.Track(nil, "generator.varfile.Validate")()

	if genCtx == nil {
		return fmt.Errorf("%w: context is nil", generator.ErrInvalidContext)
	}

	// VarsSection can be empty (no vars for component), which is valid.
	// We just won't generate anything in that case.

	return nil
}

// Generate produces the varfile content.
// For varfiles, the content is simply the VarsSection map.
func (g *Generator) Generate(ctx context.Context, genCtx *generator.GeneratorContext) (map[string]any, error) {
	defer perf.Track(nil, "generator.varfile.Generate")()

	if err := g.Validate(genCtx); err != nil {
		return nil, err
	}

	// Return nil if no vars to generate.
	if len(genCtx.VarsSection) == 0 {
		return nil, nil
	}

	// The varfile content is simply the vars section.
	// Unlike other generators that wrap content in a structure,
	// varfiles are direct key-value pairs.
	return genCtx.VarsSection, nil
}

// ConstructFilename constructs the dynamic varfile name based on the context.
// This follows the pattern: {context}-{component}.terraform.tfvars.json
// or: {context}-{folder}-{component}.terraform.tfvars.json if folder prefix exists.
func ConstructFilename(genCtx *generator.GeneratorContext) string {
	if genCtx == nil {
		return "terraform.tfvars.json"
	}
	if genCtx.StackInfo == nil {
		return "terraform.tfvars.json"
	}

	info := genCtx.StackInfo
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		return fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	}
	return fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
}
