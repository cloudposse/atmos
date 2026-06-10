package ci

import "github.com/cloudposse/atmos/pkg/ci/internal/provider"

// Context is the public alias for CI run metadata supplied by a provider.
// Consumers outside pkg/ci (e.g. cmd/git for CI checkout replacement) use
// this alias; the underlying type lives in the internal provider package.
type Context = provider.Context

// PRInfo is the public alias for pull request metadata.
type PRInfo = provider.PRInfo
