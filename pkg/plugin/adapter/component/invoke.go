package component

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
)

func (p *Provider) invoke(ctx context.Context, capability string, run func(context.Context, *native.Handle, *pluginv1.Invocation) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.Supports(capability) {
		return errUtils.ErrPluginUnsupportedExtension
	}
	handle, err := native.Start(ctx, p.executable, native.WithDirectory(p.directory), native.WithDescription(p.expected), native.WithCallTimeout(p.timeout))
	if err != nil {
		return err
	}
	defer handle.Close()
	// No ambient HTTP, browser, secret, or private-credential grants. Future host
	// adapters must provide explicit invocation-scoped grants for allowed effects.
	scope := bridge.Invocation{Namespace: p.owner + ":component:" + p.name}
	err = handle.Invoke(ctx, scope, func(call context.Context, invocation *pluginv1.Invocation) error {
		return run(call, handle, invocation)
	})
	return invocationError(err)
}

func invocationError(err error) error {
	if err == nil {
		return nil
	}
	for _, sentinel := range []error{errUtils.ErrPluginTimeout, errUtils.ErrPluginInvalidConfig, errUtils.ErrPluginProtocol, errUtils.ErrDuplicatePluginExtension, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, sentinel) {
			return err
		}
	}
	// A structured detail is the plugin's own vetted description of the failure.
	if detail, ok := detailFromStatus(err); ok {
		return DetailError(detail, status.Code(err))
	}
	switch status.Code(err) {
	case codes.Canceled:
		return errors.Join(errUtils.ErrPluginAPI, context.Canceled)
	case codes.DeadlineExceeded:
		return errors.Join(errUtils.ErrPluginAPI, context.DeadlineExceeded)
	case codes.InvalidArgument:
		return discoveryDiagnostic(err)
	case codes.Unimplemented:
		return errUtils.ErrPluginUnsupportedExtension
	default:
		// Arbitrary plugin RPC messages can disclose evaluated credentials.
		return errUtils.ErrPluginAPI
	}
}

func scopedInvocation(original, scoped *pluginv1.Invocation) *pluginv1.Invocation {
	result := &pluginv1.Invocation{}
	if original != nil {
		result = proto.Clone(original).(*pluginv1.Invocation)
	}
	result.Id, result.Attempt, result.Deadline = scoped.Id, scoped.Attempt, scoped.Deadline
	// Interactive host services are not granted by this adapter.
	result.Interactive = false
	return result
}

func (p *Provider) componentRequest(input *pluginv1.ComponentRequest, requireAddress bool) (*pluginv1.ComponentRequest, error) {
	if input == nil || !filepath.IsAbs(input.RootDirectory) || (input.Provider != "" && input.Provider != p.name) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if requireAddress && !validAddress(input.Address) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if input.Address != nil && input.Address.ComponentType != p.componentType {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if err := p.validateConfiguration(input.ProviderConfiguration); err != nil {
		return nil, err
	}
	request := proto.Clone(input).(*pluginv1.ComponentRequest)
	request.Provider = p.name
	return request, nil
}

func (p *Provider) validateOperation(ctx context.Context, handle *native.Handle, request *pluginv1.ComponentRequest) error {
	if !p.Supports(extension.ComponentValidation) {
		return nil
	}
	result, err := handle.Validation.Validate(ctx, request)
	if err != nil {
		return err
	}
	if result == nil {
		return errUtils.ErrPluginProtocol
	}
	return firstDiagnosticError(result.Diagnostics)
}

// discoveryDiagnostic accepts only the versioned discovery contract. Messages,
// unknown categories, and extra metadata never become host diagnostics. File is
// a root-relative configuration identity (including virtual generated files),
// not a path or source snippet extracted from an HCL error message.
func discoveryDiagnostic(err error) error {
	for _, detail := range status.Convert(err).Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok || info.Domain != "atmos.terragrunt.discovery.v1" || len(info.Metadata) != 1 {
			continue
		}
		file := info.Metadata["file"]
		if !validDiscoveryFile(file) {
			continue
		}
		var message string
		switch info.Reason {
		case "MISSING_TERRAFORM_SOURCE":
			message = "inline Terraform modules require terraform.source"
		case "NONLOCAL_SOURCE":
			message = "discovery requires a local source"
		case "EVALUATION_FAILED":
			message = "configuration evaluation failed"
		default:
			continue
		}
		return fmt.Errorf("%w: Terragrunt discovery in %q: %s", errUtils.ErrPluginAPI, file, message)
	}
	return errUtils.ErrPluginAPI
}

func validDiscoveryFile(file string) bool {
	const maxDiagnosticPathBytes = 4096
	return len(file) <= maxDiagnosticPathBytes && filepath.IsLocal(file) &&
		!strings.ContainsAny(file, "\\\r\n\x00") && filepath.ToSlash(filepath.Clean(file)) == file
}
