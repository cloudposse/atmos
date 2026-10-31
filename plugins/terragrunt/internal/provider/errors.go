package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// rpcFailure is a plugin error that already carries the structured description the host renders.
// It becomes a gRPC status whose details hold the description.
type rpcFailure struct {
	failure *component.Failure
	code    codes.Code
}

func (f *rpcFailure) Error() string { return f.failure.Proto().Summary }

// GRPCStatus implements the interface the gRPC server uses to build the response status.
func (f *rpcFailure) GRPCStatus() *status.Status {
	return status.Convert(f.failure.Err(f.code))
}

// rpcError converts an evaluation error to an RPC error. Evaluator payloads are never copied into
// a message the host prints: HCL diagnostics can contain secrets, so only the structured fields
// that carry no evaluated values are shown, and detail text appears only when it is marked safe.
func rpcError(err error) error {
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "Terragrunt evaluation canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Terragrunt evaluation timed out")
	}
	var failure *rpcFailure
	if errors.As(err, &failure) {
		return failure
	}
	return &rpcFailure{failure: failureFor(err), code: codeFor(err)}
}

func codeFor(err error) codes.Code {
	var problem *model.Problem
	if errors.As(err, &problem) {
		switch problem.Code {
		case model.CodeUnsupportedSetting, model.CodeUnsupportedHook, model.CodeWorkspace:
			return codes.Unimplemented
		}
		return codes.InvalidArgument
	}
	if errors.Is(err, ErrUnsupported) || errors.Is(err, engine.ErrUnsupportedExecution) {
		return codes.Unimplemented
	}
	return codes.InvalidArgument
}

// failureFor describes err. A model.Problem keeps its own fields; anything else becomes a generic
// failure whose message stays hidden.
func failureFor(err error) *component.Failure {
	var problem *model.Problem
	if errors.As(err, &problem) {
		failure := component.NewFailure(problem.Code, problem.Summary).Key(problem.Key)
		if problem.Safe {
			failure.SafeDetail(problem.Detail)
		} else if problem.Detail != "" {
			failure.Detail(problem.Detail)
		}
		for _, hint := range problem.Hints {
			failure.Hint(hint)
		}
		if problem.File != "" {
			failure.At(problem.File, uint32(max(problem.Line, 0)), uint32(max(problem.Column, 0))) //nolint:gosec // Line and column are small non-negative positions.
		}
		return failure
	}
	if errors.Is(err, ErrUnsupported) {
		return component.NewFailure(model.CodeUnsupportedSetting, "Unsupported native Terragrunt configuration").
			SafeDetail("The request uses a mode, option, or path the native Terragrunt adaptation does not support.").
			Hint("Check `components.terraform.terragrunt` in atmos.yaml: only `mode: native` is supported.")
	}
	return component.NewFailure(model.CodeInternal, "Terragrunt evaluation failed").Detail(err.Error())
}

// scoped names the operation, stack, and component on a failure and makes its file project relative.
func scoped(err error, operation, root string, address *pluginv1.ComponentAddress) error {
	var failure *rpcFailure
	if !errors.As(err, &failure) {
		return err
	}
	detail := failure.failure.Proto()
	updated := component.NewFailure(detail.Code, detail.Summary).Operation(operation).Key(detail.ConfigKey)
	if detail.UserSafe {
		updated.SafeDetail(detail.Detail)
	} else if detail.Detail != "" {
		updated.Detail(detail.Detail)
	}
	for _, hint := range detail.Hints {
		updated.Hint(hint)
	}
	if detail.Location != nil {
		updated.At(projectRelative(root, detail.Location.File), detail.Location.Line, detail.Location.Column)
	}
	if address != nil {
		updated.Unit(address.Stack, address.Component)
	}
	return &rpcFailure{failure: updated, code: failure.code}
}

// projectRelative returns file relative to the project root, or file unchanged when it lies elsewhere.
func projectRelative(root, file string) string {
	if root == "" || file == "" || !filepath.IsAbs(file) {
		return file
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) {
		return file
	}
	return filepath.ToSlash(rel)
}

// unitDiagnostic reports one failed unit as a per-unit diagnostic that does not fail discovery.
func unitDiagnostic(root string, failure model.UnitFailure) *pluginv1.Diagnostic {
	address := &pluginv1.ComponentAddress{Stack: failure.Stack, Component: failure.Name, ComponentType: "terraform"}
	var rpc *rpcFailure
	if errors.As(scoped(rpcError(failure.Err), "discover", root, address), &rpc) {
		return rpc.failure.Diagnostic()
	}
	return component.NewFailure(model.CodeInternal, fmt.Sprintf("Terragrunt evaluation failed for %s", failure.Name)).Unit(failure.Stack, failure.Name).Operation("discover").Diagnostic()
}

// settingProblem names an atmos.yaml setting under components.terraform.terragrunt and how to fix it.
func settingProblem(key, summary, hint string) *model.Problem {
	return &model.Problem{Code: model.CodeUnsupportedSetting, Summary: summary, Hints: []string{hint}, File: "atmos.yaml", Key: "components.terraform.terragrunt." + key, Safe: true, Err: ErrUnsupported}
}

// problemFor gives an error without a description a file, so the user can find the unit.
func problemFor(err error, file string) error {
	var problem *model.Problem
	if errors.As(err, &problem) || file == "" {
		return err
	}
	return &model.Problem{
		Code: model.CodeInvalidSource, Summary: "The unit cannot be described to Atmos", File: file, Safe: false, Detail: err.Error(), Err: err,
		Hints: []string{"Check that the unit's `terraform.source` and its dependencies are inside the project root."},
	}
}
