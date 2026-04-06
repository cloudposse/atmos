package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// Provider represents a CI/CD provider (GitHub Actions, GitLab CI, etc.).
type Provider = provider.Provider

// BaseResolution contains the resolved base commit for affected detection.
type BaseResolution = provider.BaseResolution
