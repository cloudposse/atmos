package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// Provider represents a CI/CD provider (GitHub Actions, GitLab CI, etc.).
type Provider = provider.Provider

// BaseResolution contains the resolved base commit for affected detection.
type BaseResolution = provider.BaseResolution

// SBOMReport is a serialized SBOM ready for publication through a CI provider.
type SBOMReport = provider.SBOMReport

// SBOMUpload identifies a CI-provider publication of an SBOM.
type SBOMUpload = provider.SBOMUpload

// SBOMUploader is an optional CI capability for publishing an SBOM.
type SBOMUploader = provider.SBOMUploader
