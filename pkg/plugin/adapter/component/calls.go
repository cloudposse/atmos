package component

import (
	"context"
	"path/filepath"

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
			return err
		}
		return checkPreparation(result)
	})
	if err != nil {
		return nil, err
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
		if len(result.Diagnostics) != 0 {
			return errUtils.ErrPluginInvalidConfig
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
