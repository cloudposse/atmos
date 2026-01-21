package function

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// EnvFunction implements the !env YAML function.
// It retrieves environment variables with optional defaults.
type EnvFunction struct {
	BaseFunction
}

// NewEnvFunction creates a new EnvFunction.
func NewEnvFunction() *EnvFunction {
	defer perf.Track(nil, "function.NewEnvFunction")()

	return &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "env",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the !env function.
// Syntax: !env VAR_NAME [default_value]
// Returns the environment variable value, or the default if not set.
func (f *EnvFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.EnvFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return "", fmt.Errorf("%w: !env requires at least one argument", ErrInvalidArguments)
	}

	envVarName, envVarDefault, err := f.parseEnvArgs(args)
	if err != nil {
		return "", err
	}

	// Check execution context for component env section first.
	if val := f.getFromContext(envVarName, execCtx); val != "" {
		return val, nil
	}

	// Fall back to OS environment variables.
	if val, exists := os.LookupEnv(envVarName); exists {
		return val, nil
	}

	// Return default value if provided.
	return envVarDefault, nil
}

// parseEnvArgs parses the arguments for the !env function.
func (f *EnvFunction) parseEnvArgs(args string) (envVarName, envVarDefault string, err error) {
	defer perf.Track(nil, "function.EnvFunction.parseEnvArgs")()

	parts, err := splitStringByDelimiter(args, ' ')
	if err != nil {
		return "", "", errors.Join(ErrInvalidArguments, err)
	}

	switch len(parts) {
	case 1:
		return strings.TrimSpace(parts[0]), "", nil
	case 2:
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
	default:
		return "", "", fmt.Errorf("%w: !env accepts 1 or 2 arguments, got %d", ErrInvalidArguments, len(parts))
	}
}

// getFromContext gets a value from the execution context.
func (f *EnvFunction) getFromContext(envVarName string, execCtx *ExecutionContext) string {
	if execCtx == nil || execCtx.Env == nil {
		return ""
	}
	return execCtx.Env[envVarName]
}

// stringSplitter handles splitting strings by delimiter with quote support.
type stringSplitter struct {
	result    []string
	current   strings.Builder
	inQuotes  bool
	quoteChar rune
	delim     rune
}

// splitStringByDelimiter splits a string by delimiter, respecting quoted sections.
func splitStringByDelimiter(input string, delim rune) ([]string, error) {
	defer perf.Track(nil, "function.splitStringByDelimiter")()

	s := &stringSplitter{delim: delim}

	for _, r := range input {
		s.processRune(r)
	}

	if s.current.Len() > 0 {
		s.result = append(s.result, s.current.String())
	}

	if s.inQuotes {
		return nil, fmt.Errorf("%w: %s", ErrUnclosedQuote, input)
	}

	return s.result, nil
}

// processRune handles a single rune during string splitting.
func (s *stringSplitter) processRune(r rune) {
	switch {
	case r == '"' || r == '\'':
		s.handleQuote(r)
	case r == s.delim && !s.inQuotes:
		if s.current.Len() > 0 {
			s.result = append(s.result, s.current.String())
			s.current.Reset()
		}
	default:
		s.current.WriteRune(r)
	}
}

// handleQuote handles quote characters during string splitting.
func (s *stringSplitter) handleQuote(r rune) {
	if !s.inQuotes {
		s.inQuotes = true
		s.quoteChar = r
		return
	}
	if r == s.quoteChar {
		s.inQuotes = false
		s.quoteChar = 0
		return
	}
	s.current.WriteRune(r)
}
