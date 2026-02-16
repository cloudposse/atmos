// Package ci provides CI/CD provider abstractions and integrations.
package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// Provider represents a CI/CD provider (GitHub Actions, GitLab CI, etc.).
type Provider = provider.Provider

// OutputWriter writes CI outputs (environment variables, job summaries, etc.).
type OutputWriter = provider.OutputWriter
