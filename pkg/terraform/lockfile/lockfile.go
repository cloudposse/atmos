// Package lockfile parses Terraform/OpenTofu dependency lock files
// (.terraform.lock.hcl) to extract the exact set of providers and versions a
// component depends on. The lock file is the authoritative source of resolved
// provider versions (including transitive providers pulled in by child modules),
// which makes it the right input for eagerly warming the provider cache across
// platforms.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Name is the conventional filename of the Terraform/OpenTofu dependency lock
// file. Both tools write the same filename.
const Name = ".terraform.lock.hcl"

// Provider is a single provider dependency recorded in a lock file.
type Provider struct {
	// Source is the fully-qualified provider source address recorded as the
	// provider block label, e.g. "registry.terraform.io/hashicorp/aws".
	Source string
	// Version is the exact locked version, e.g. "5.95.0".
	Version string
}

// providerSchema matches the provider blocks in a lock file. Lock files only
// contain provider blocks, but PartialContent tolerates anything else.
var providerSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "provider", LabelNames: []string{"source"}},
	},
}

// versionSchema matches the version attribute inside a provider block.
var versionSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "version", Required: true},
	},
}

// ParseFile reads and parses the lock file at path, returning its providers.
// A missing file is reported via errUtils.ErrFileNotFound so callers can decide
// whether to generate one (e.g. by running `terraform init`) before warming.
func ParseFile(path string) ([]Provider, error) {
	defer perf.Track(nil, "lockfile.ParseFile")()

	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrFileNotFound, path)
		}
		return nil, fmt.Errorf("%w: reading lock file %s: %w", errUtils.ErrInvalidConfig, path, err)
	}
	return Parse(src, path)
}

// ParseDir parses the lock file (.terraform.lock.hcl) in dir. It is a
// convenience wrapper around ParseFile.
func ParseDir(dir string) ([]Provider, error) {
	defer perf.Track(nil, "lockfile.ParseDir")()

	return ParseFile(filepath.Join(dir, Name))
}

// Parse parses lock file content; filename is used only for diagnostics.
func Parse(src []byte, filename string) ([]Provider, error) {
	defer perf.Track(nil, "lockfile.Parse")()

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(src, filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w: parsing lock file %s: %s", errUtils.ErrInvalidConfig, filename, diags.Error())
	}

	content, _, diags := file.Body.PartialContent(providerSchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w: reading lock file %s: %s", errUtils.ErrInvalidConfig, filename, diags.Error())
	}

	providers := make([]Provider, 0, len(content.Blocks))
	for _, block := range content.Blocks {
		version, err := providerVersion(block, filename)
		if err != nil {
			return nil, err
		}
		providers = append(providers, Provider{Source: block.Labels[0], Version: version})
	}
	return providers, nil
}

// providerVersion extracts the version attribute from a provider block.
func providerVersion(block *hcl.Block, filename string) (string, error) {
	attrs, _, diags := block.Body.PartialContent(versionSchema)
	if diags.HasErrors() {
		return "", fmt.Errorf("%w: provider %q in %s: %s", errUtils.ErrInvalidConfig, block.Labels[0], filename, diags.Error())
	}

	val, diags := attrs.Attributes["version"].Expr.Value(nil)
	if diags.HasErrors() {
		return "", fmt.Errorf("%w: provider %q version in %s: %s", errUtils.ErrInvalidConfig, block.Labels[0], filename, diags.Error())
	}
	if val.Type() != cty.String || val.IsNull() {
		return "", fmt.Errorf("%w: provider %q in %s has a non-string version", errUtils.ErrInvalidConfig, block.Labels[0], filename)
	}
	return val.AsString(), nil
}
