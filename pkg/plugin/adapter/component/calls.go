package component

import (
	"context"
	"math"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
)

// Discover resolves instances without replacing their registered execution engine.
func (p *Provider) Discover(ctx context.Context, input *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	defer perf.Track(nil, "componentadapter.Discover")()

	if input == nil || !filepath.IsAbs(input.RootDirectory) || (input.Provider != "" && input.Provider != p.name) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if err := p.validateConfiguration(input.ProviderConfiguration); err != nil {
		return nil, err
	}
	request := proto.Clone(input).(*pluginv1.DiscoverComponentsRequest)
	request.Provider = p.name
	var result *pluginv1.DiscoverComponentsResponse
	err := p.invoke(ctx, extension.ComponentDiscovery, func(call context.Context, handle *native.Handle, invocation *pluginv1.Invocation) error {
		request.Invocation = scopedInvocation(request.Invocation, invocation)
		validation := &pluginv1.ComponentRequest{Invocation: request.Invocation, Provider: request.Provider, RootDirectory: request.RootDirectory, Operation: request.Operation, Effects: request.Effects, ProviderConfiguration: request.ProviderConfiguration}
		if err := p.validateOperation(call, handle, validation); err != nil {
			return err
		}
		var err error
		result, err = handle.Discovery.Discover(call, request)
		if err != nil {
			return err
		}
		return p.checkDiscovery(result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Validate returns explicit diagnostics for rendering through the host's masked I/O.
func (p *Provider) Validate(ctx context.Context, input *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
	defer perf.Track(nil, "componentadapter.Validate")()

	request, err := p.componentRequest(input, false)
	if err != nil {
		return nil, err
	}
	var result *pluginv1.ValidateResponse
	err = p.invoke(ctx, extension.ComponentValidation, func(call context.Context, handle *native.Handle, invocation *pluginv1.Invocation) error {
		request.Invocation = scopedInvocation(request.Invocation, invocation)
		var err error
		result, err = handle.Validation.Validate(call, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Prepare reevaluates a selected instance after semantic validation. It never
// executes the returned engine or writes the returned artifacts.
func (p *Provider) Prepare(ctx context.Context, input *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	defer perf.Track(nil, "componentadapter.Prepare")()

	request, err := p.componentRequest(input, true)
	if err != nil {
		return nil, err
	}
	var result *pluginv1.PrepareComponentResponse
	err = p.invoke(ctx, extension.ComponentPreparation, func(call context.Context, handle *native.Handle, invocation *pluginv1.Invocation) error {
		request.Invocation = scopedInvocation(request.Invocation, invocation)
		if err := p.validateOperation(call, handle, request); err != nil {
			return err
		}
		var err error
		result, err = handle.Preparation.Prepare(call, request)
		if err != nil {
			// A failing call can still return the response it produced. Whatever it carries is registered
			// for masking before the failure is rendered; the call's own error is the one reported.
			if result != nil {
				_ = registerResponseSecrets(result)
			}
			return err
		}
		return checkPreparationResponse(result, request.GetOperation().GetMode())
	})
	if err != nil {
		return nil, err
	}
	// The registry time is observed while this call's span is open, so it is a child of that span.
	if nanos := result.GetRegistryResolutionNanos(); nanos > 0 && nanos <= uint64(math.MaxInt64) {
		perf.ObserveExternal("atmosbridge.ResolveRegistrySource", time.Duration(nanos))
	}
	return result, nil
}

// ResolveState obtains backend/workspace identity without discovery or preparation.
func (p *Provider) ResolveState(ctx context.Context, input *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	defer perf.Track(nil, "componentadapter.ResolveState")()

	request, err := p.componentRequest(input, true)
	if err != nil {
		return nil, err
	}
	var result *pluginv1.ResolveComponentStateResponse
	err = p.invoke(ctx, extension.ComponentState, func(call context.Context, handle *native.Handle, invocation *pluginv1.Invocation) error {
		request.Invocation = scopedInvocation(request.Invocation, invocation)
		if err := p.validateOperation(call, handle, request); err != nil {
			return err
		}
		var err error
		result, err = handle.State.Resolve(call, request)
		if err != nil {
			return err
		}
		if result == nil {
			return errUtils.ErrPluginProtocol
		}
		if err := firstDiagnosticError(result.Diagnostics); err != nil {
			return err
		}
		if !validState(result.State) || !validSources(result.Sources) {
			return errUtils.ErrPluginProtocol
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// registerResponseInputs registers the sensitive inputs of a preparation response for masking. A response
// without inputs registers nothing, and a missing response is reported by the checks that follow.
func registerResponseInputs(result *pluginv1.PrepareComponentResponse) error {
	if result.GetInputs() == nil {
		return nil
	}
	_, err := DecodeInputs(result.GetInputs())
	return err
}

// registerResponseSecrets registers everything a preparation response carries that must be masked, before any
// diagnostic is rendered: the sensitive inputs, then the module source authorization. A value that cannot be
// decoded is a protocol failure that takes precedence over the response's diagnostics, which could otherwise
// print what was never registered; the inputs are checked first.
func registerResponseSecrets(result *pluginv1.PrepareComponentResponse) error {
	if err := registerResponseInputs(result); err != nil {
		return err
	}
	_, err := DecodeString(result.GetModuleSourceAuthorization())
	return err
}

func checkPreparationResponse(result *pluginv1.PrepareComponentResponse, mode string) error {
	if mode != "iam-probe" {
		// The sensitive inputs are registered for masking before anything else touches the response.
		// Warnings are not printed while the response is checked: the configuration session prints
		// them once per invocation through its deduper, and printing them here as well would repeat
		// every warning, on the first preparation and on each later one. Invalid inputs return
		// their decode error before diagnostics can expose values that were not registered.
		if result == nil {
			return errUtils.ErrPluginProtocol
		}
		if err := registerResponseSecrets(result); err != nil {
			return err
		}
		failures := proto.Clone(result).(*pluginv1.PrepareComponentResponse)
		failures.Diagnostics = errorDiagnostics(failures.Diagnostics)
		return checkPreparation(failures)
	}
	if result == nil {
		return errUtils.ErrPluginProtocol
	}
	// IAM is resolved before module evaluation. This response may carry only the
	// role settings and diagnostics/logs, never a prepared module or artifacts.
	remaining := proto.Clone(result).(*pluginv1.PrepareComponentResponse)
	remaining.Iam = nil
	remaining.Diagnostics = nil
	remaining.Logs = nil
	if proto.Size(remaining) != 0 {
		return errUtils.ErrPluginProtocol
	}
	// Validate and register the token before diagnostic details can reach the logger.
	if _, err := DecodeString(result.GetIam().GetWebIdentityToken()); err != nil {
		return err
	}
	return firstDiagnosticError(result.Diagnostics)
}
