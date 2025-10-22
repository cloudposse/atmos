# Atmos Deployments - SBOM Generation

This document describes the Software Bill of Materials (SBOM) generation strategy using the component registry pattern.

## Overview

SBOM (Software Bill of Materials) generation follows the **component registry pattern** - each component type is responsible for generating its own SBOM, and the component registry aggregates them.

**SBOM Format**: CycloneDX (primary), SPDX (secondary)

## Component Registry Pattern

### SBOMGenerator Interface

Components opt into SBOM generation by implementing the `SBOMGenerator` interface:

```go
// pkg/component/interface.go
type SBOMGenerator interface {
    GenerateSBOM(ctx context.Context) (*cdx.BOM, error)
    SupportsSBOM() bool
}

type Component interface {
    Type() ComponentType
    Name() string
    Execute(ctx context.Context) error
    // Optional SBOM generation
    SBOMGenerator  // Embedded interface
}
```

### Component Implementations

Each component type implements SBOM generation:

```go
// pkg/component/nixpack.go
type NixpackComponent struct {
    name   string
    config *NixpackConfig
}

func (n *NixpackComponent) SupportsSBOM() bool {
    return true
}

func (n *NixpackComponent) GenerateSBOM(ctx context.Context) (*cdx.BOM, error) {
    bom := cdx.NewBOM()
    bom.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type: cdx.ComponentTypeContainer,
            Name: n.config.Image.Name,
            Version: n.config.Image.Tag,
        },
    }

    // Add container image component
    bom.Components = &[]cdx.Component{
        {
            Type:    cdx.ComponentTypeContainer,
            Name:    n.config.Image.Name,
            Version: n.config.Image.Digest,
            Hashes:  []cdx.Hash{{Algorithm: cdx.HashAlgoSHA256, Value: n.config.Image.Digest}},
        },
    }

    // Add application dependencies from nixpacks build
    appDeps, err := n.parseNixpacksDependencies(ctx)
    if err != nil {
        return nil, err
    }
    *bom.Components = append(*bom.Components, appDeps...)

    // Add Nix packages
    for _, pkg := range n.config.Pkgs {
        *bom.Components = append(*bom.Components, cdx.Component{
            Type:    cdx.ComponentTypeLibrary,
            Name:    pkg,
            Supplier: &cdx.OrganizationalEntity{Name: "nixpkgs"},
        })
    }

    return bom, nil
}

// pkg/component/terraform.go
type TerraformComponent struct {
    name   string
    config *TerraformConfig
}

func (t *TerraformComponent) SupportsSBOM() bool {
    return true
}

func (t *TerraformComponent) GenerateSBOM(ctx context.Context) (*cdx.BOM, error) {
    bom := cdx.NewBOM()

    // Parse Terraform files for module dependencies
    modulePath := t.config.ComponentPath
    modules, err := ParseTerraformModule(modulePath)
    if err != nil {
        return nil, err
    }

    // Add module dependencies
    for _, mod := range modules.ModuleCalls {
        *bom.Components = append(*bom.Components, cdx.Component{
            Type:    cdx.ComponentTypeLibrary,
            Name:    mod.Name,
            Version: mod.Version,
            Purl:    fmt.Sprintf("pkg:terraform/%s@%s", mod.Source, mod.Version),
        })
    }

    // Parse .terraform.lock.hcl for provider versions
    providers, err := ParseTerraformLockFile(filepath.Join(modulePath, ".terraform.lock.hcl"))
    if err == nil {
        for _, provider := range providers {
            *bom.Components = append(*bom.Components, cdx.Component{
                Type:    cdx.ComponentTypeLibrary,
                Name:    provider.Name,
                Version: provider.Version,
                Hashes:  []cdx.Hash{{Algorithm: cdx.HashAlgoSHA256, Value: provider.Hash}},
                Purl:    fmt.Sprintf("pkg:terraform-provider/%s@%s", provider.Name, provider.Version),
            })
        }
    }

    return bom, nil
}

// pkg/component/helmfile.go
type HelmfileComponent struct {
    name   string
    config *HelmfileConfig
}

func (h *HelmfileComponent) SupportsSBOM() bool {
    return true
}

func (h *HelmfileComponent) GenerateSBOM(ctx context.Context) (*cdx.BOM, error) {
    bom := cdx.NewBOM()

    // Parse helmfile.yaml for chart dependencies
    helmfile, err := ParseHelmfile(h.config.HelmfilePath)
    if err != nil {
        return nil, err
    }

    for _, release := range helmfile.Releases {
        *bom.Components = append(*bom.Components, cdx.Component{
            Type:    cdx.ComponentTypeLibrary,
            Name:    release.Name,
            Version: release.Version,
            Purl:    fmt.Sprintf("pkg:helm/%s@%s", release.Chart, release.Version),
        })
    }

    return bom, nil
}
```

### Component Registry Aggregation

The component registry aggregates SBOMs from all components:

```go
// pkg/component/registry.go
type Registry struct {
    components map[string]Component
}

func (r *Registry) GenerateDeploymentSBOM(ctx context.Context, deployment, target string) (*cdx.BOM, error) {
    aggregatedBOM := cdx.NewBOM()
    aggregatedBOM.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type:    cdx.ComponentTypeApplication,
            Name:    deployment,
            Version: target,
        },
    }

    for name, component := range r.components {
        generator, ok := component.(SBOMGenerator)
        if !ok || !generator.SupportsSBOM() {
            continue
        }

        bom, err := generator.GenerateSBOM(ctx)
        if err != nil {
            log.Warn("Failed to generate SBOM for component", "component", name, "error", err)
            continue
        }

        // Merge component SBOM into aggregated SBOM
        if bom.Components != nil {
            if aggregatedBOM.Components == nil {
                aggregatedBOM.Components = &[]cdx.Component{}
            }
            *aggregatedBOM.Components = append(*aggregatedBOM.Components, *bom.Components...)
        }
    }

    return aggregatedBOM, nil
}
```

## Terraform HCL Parsing

Extract SBOM data from Terraform files using HashiCorp's official libraries:

```go
// pkg/sbom/terraform_parser.go
package sbom

import (
    "github.com/hashicorp/terraform-config-inspect/tfconfig"
    "github.com/hashicorp/hcl/v2/hclparse"
)

type TerraformModuleInfo struct {
    ModuleCalls      []ModuleCall
    RequiredProviders map[string]ProviderRequirement
}

type ModuleCall struct {
    Name    string
    Source  string
    Version string
}

func ParseTerraformModule(modulePath string) (*TerraformModuleInfo, error) {
    module, diags := tfconfig.LoadModule(modulePath)
    if diags.HasErrors() {
        return nil, diags.Err()
    }

    info := &TerraformModuleInfo{
        ModuleCalls:      make([]ModuleCall, 0),
        RequiredProviders: make(map[string]ProviderRequirement),
    }

    // Extract module calls
    for name, mod := range module.ModuleCalls {
        info.ModuleCalls = append(info.ModuleCalls, ModuleCall{
            Name:    name,
            Source:  mod.Source,
            Version: mod.Version,
        })
    }

    // Extract required providers
    for name, provider := range module.RequiredProviders {
        info.RequiredProviders[name] = ProviderRequirement{
            Name:    name,
            Source:  provider.Source,
            Version: provider.VersionConstraints,
        }
    }

    return info, nil
}

func ParseTerraformLockFile(lockFilePath string) ([]ProviderLockEntry, error) {
    parser := hclparse.NewParser()
    f, diags := parser.ParseHCLFile(lockFilePath)
    if diags.HasErrors() {
        return nil, diags.Errs()[0]
    }

    // Parse provider blocks
    var entries []ProviderLockEntry
    // ... HCL parsing logic ...

    return entries, nil
}
```

## SBOM CLI Commands

```bash
# Generate SBOM for deployment
atmos deployment sbom payment-service --target dev

# Output formats
atmos deployment sbom payment-service --target dev --format cyclonedx-json > sbom.cdx.json
atmos deployment sbom payment-service --target dev --format spdx-json > sbom.spdx.json

# Include in release
atmos deployment release payment-service --target dev --sbom
# SBOM attached to release record automatically
```

## SBOM in Release Records

SBOMs are automatically included in release records:

```yaml
# releases/api/dev/release-abc123.yaml
release:
  id: "abc123"
  deployment: api
  target: dev

  artifacts:
    api:
      type: nixpack
      digest: "sha256:1234567890abcdef..."
      sbom:
        - format: "cyclonedx-json"
          digest: "sha256:sbom123..."
          path: "releases/api/dev/sbom-abc123.cdx.json"
```

## See Also

- **[overview.md](./overview.md)** - Core concepts
- **[nixpacks.md](./nixpacks.md)** - Container build SBOM
- **[configuration.md](./configuration.md)** - SBOM settings
