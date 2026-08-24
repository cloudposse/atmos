package function

import (
	"context"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/utils"
)

// SecretFunction implements the secret function for resolving declared secrets from their
// configured backend. It delegates to pkg/secrets, which enforces the declarative registry,
// the mask-without-retrieval rule on inspection commands, and automatic masker registration.
type SecretFunction struct {
	BaseFunction
}

// NewSecretFunction creates a new secret function handler.
func NewSecretFunction() *SecretFunction {
	defer perf.Track(nil, "function.NewSecretFunction")()

	return &SecretFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagSecret,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the secret function.
// Usage:
//
//	!secret NAME                       - Resolve a declared secret value.
//	!secret NAME | path ".a.b"         - Extract a nested value from a structured secret.
//	!secret NAME | default "dev-key"   - Fall back to a default when the secret is missing.
func (f *SecretFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.SecretFunction.Execute")()

	log.Debug("Executing secret function", "args", args)

	if execCtx == nil || execCtx.AtmosConfig == nil {
		return nil, ErrExecutionFailed
	}

	// pkg/secrets.Resolve parses the full `!secret ...` form, so reconstruct it from the tag.
	input := utils.AtmosYamlFuncSecret + " " + args
	return secrets.Resolve(execCtx.AtmosConfig, input, execCtx.Stack, execCtx.StackInfo)
}
