package auth

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
)

const maxCredentialEvents = 1024

type invocation struct {
	handle  *native.Handle
	request *pluginv1.Invocation
}

func (d *Definition) invoke(ctx context.Context, name, realm string, settings map[string]any, operation func(*invocation) error) error {
	namespace, err := d.sessionNamespace(name, realm, settings)
	if err != nil {
		return err
	}
	return d.transaction(ctx, namespace, func(ctx context.Context) error {
		handle, err := native.Start(ctx, d.Executable, native.WithDirectory(d.Directory), native.WithDescription(d.Expected), native.WithCallTimeout(d.Timeout))
		if err != nil {
			return err
		}
		defer handle.Close()
		browser, interactive := defaultBrowser(&d.Services)
		transport := d.Services.HTTP
		if transport == nil {
			transport = api.HTTPService{}
		}
		scope := bridge.Invocation{Browser: browser, Interactive: interactive, HTTP: transport, Keyring: d.Services.Keyring, Namespace: namespace}
		return translateError(handle.Invoke(ctx, scope, func(ctx context.Context, request *pluginv1.Invocation) error {
			return operation(&invocation{handle: handle, request: request})
		}))
	})
}

func toStruct(value map[string]any) (*structpb.Struct, error) {
	result, err := structpb.NewStruct(value)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	return result, nil
}

func receiveCredentials(stream grpc.ServerStreamingClient[pluginv1.AuthenticateEvent]) (*pluginv1.Credentials, error) {
	// Progress is bounded and not blindly printed: plugin text can contain secrets.
	for range maxCredentialEvents {
		event, err := stream.Recv()
		if err != nil {
			return nil, errors.Join(errUtils.ErrPluginProtocol, err)
		}
		if event.GetResult() == nil {
			continue
		}
		result := event.GetResult()
		if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
			return nil, errUtils.ErrPluginProtocol
		}
		return result, nil
	}
	return nil, errUtils.ErrPluginProtocol
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	// Interruption and deadlines are already classified by the runtime; keep their exit status and hints.
	var exit errUtils.ExitCodeError
	if errors.As(err, &exit) || errors.Is(err, errUtils.ErrPluginTimeout) {
		return err
	}
	switch status.Code(err) {
	case codes.FailedPrecondition:
		return errUtils.ErrPluginInteractionRequired
	case codes.Canceled:
		return errors.Join(errUtils.ErrPluginAPI, context.Canceled)
	case codes.DeadlineExceeded:
		return errors.Join(errUtils.ErrPluginAPI, context.DeadlineExceeded)
	case codes.Unauthenticated:
		return errUtils.ErrAuthenticationFailed
	default:
		return translateLocalError(err)
	}
}

func translateLocalError(err error) error {
	for _, cause := range []error{errUtils.ErrPluginProtocol, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			return cause
		}
	}
	return errUtils.ErrPluginAPI
}

// logout keeps local deletion inside the session lock even if the plugin crashes,
// revocation fails, or the invocation is canceled after acquiring the lock.
func (d *Definition) logout(ctx context.Context, name, realm string, settings map[string]any, operation func(*invocation) error) error {
	namespace, err := d.sessionNamespace(name, realm, settings)
	if err != nil {
		return err
	}
	return d.transaction(ctx, namespace, func(ctx context.Context) error {
		remote := d.invoke(ctx, name, realm, settings, operation)
		return errors.Join(remote, bridge.ClearPrivateSession(d.Services.Keyring, namespace))
	})
}
