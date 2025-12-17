package function

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

// ExecFunction implements the exec function for shell command execution.
type ExecFunction struct {
	BaseFunction
}

// NewExecFunction creates a new exec function handler.
func NewExecFunction() *ExecFunction {
	defer perf.Track(nil, "function.NewExecFunction")()

	return &ExecFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagExec,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the exec function.
// Usage:
//
//	!exec command args...   - Execute shell command and return output
//
// If the output is valid JSON, it will be parsed and returned as the corresponding type.
// Otherwise, the raw string output is returned.
func (f *ExecFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.ExecFunction.Execute")()

	log.Debug("Executing exec function", "args", args)

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, ErrInvalidArguments
	}

	res, err := utils.ExecuteShellAndReturnOutput(args, YAMLTag(TagExec)+" "+args, ".", os.Environ(), false)
	if err != nil {
		return nil, err
	}

	// Try to parse as JSON.
	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		log.Debug("Output is not JSON, returning as string", "error", err)
		return res, nil
	}

	return decoded, nil
}
