package function

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

// EnvFunction implements the env function for environment variable lookup.
type EnvFunction struct {
	BaseFunction
}

// NewEnvFunction creates a new env function handler.
func NewEnvFunction() *EnvFunction {
	defer perf.Track(nil, "function.NewEnvFunction")()

	return &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagEnv,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the env function.
// Usage:
//
//	!env VAR_NAME           - Get environment variable, return empty string if not set
//	!env VAR_NAME default   - Get environment variable, return default if not set
func (f *EnvFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.EnvFunction.Execute")()

	log.Debug("Executing env function", "args", args)

	args = strings.TrimSpace(args)
	if args == "" {
		return "", fmt.Errorf("%w: env function requires at least one argument", ErrInvalidArguments)
	}

	var envVarName string
	var envVarDefault string

	parts, err := utils.SplitStringByDelimiter(args, ' ')
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidArguments, args)
	}

	switch len(parts) {
	case 2:
		envVarName = strings.TrimSpace(parts[0])
		envVarDefault = strings.TrimSpace(parts[1])
	case 1:
		envVarName = strings.TrimSpace(parts[0])
	default:
		return "", fmt.Errorf("%w: env function accepts 1 or 2 arguments, got %d", ErrInvalidArguments, len(parts))
	}

	// Check the component's env section from stack manifests first.
	if execCtx != nil && execCtx.StackInfo != nil {
		if envSection := execCtx.StackInfo.GetComponentEnvSection(); envSection != nil {
			if val, exists := envSection[envVarName]; exists {
				return fmt.Sprintf("%v", val), nil
			}
		}
	}

	// Fall back to OS environment variables.
	if res, exists := os.LookupEnv(envVarName); exists {
		return res, nil
	}

	if envVarDefault != "" {
		return envVarDefault, nil
	}

	return "", nil
}
