package function

import (
	"context"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// LiteralFunction implements the literal function for preserving values as-is.
// This bypasses template processing to preserve template-like syntax ({{...}}, ${...})
// for downstream tools like Terraform, Helm, and ArgoCD.
type LiteralFunction struct {
	BaseFunction
}

// NewLiteralFunction creates a new literal function handler.
func NewLiteralFunction() *LiteralFunction {
	defer perf.Track(nil, "function.NewLiteralFunction")()

	return &LiteralFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagLiteral,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge, // Must run before template processing.
		},
	}
}

// Execute processes the literal function.
// Usage:
//
//	!literal "{{external.email}}"
//	!literal "{{ .Values.ingress.class }}"
//	!literal |
//	  #!/bin/bash
//	  echo "Hello ${USER}"
//
// The function returns the argument exactly as provided, preserving any
// template-like syntax that would otherwise be processed by Atmos.
func (f *LiteralFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.LiteralFunction.Execute")()

	log.Debug("Executing literal function", "args", args)

	// Return the value as-is, preserving any template syntax.
	// The args string contains whatever follows the !literal tag.
	return strings.TrimSpace(args), nil
}
