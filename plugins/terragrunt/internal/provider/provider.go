// Package provider adapts Terragrunt evaluation to Atmos component services.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"strings"

	ctyjson "github.com/zclconf/go-cty/cty/json"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

var ErrUnsupported = errors.New("unsupported native Terragrunt configuration")

// Evaluator is the only dependency that reads or evaluates HCL.
type Evaluator func(context.Context, model.Request) (*model.Resolution, error)

// Provider implements the four independent configuration services.
type Provider struct {
	pluginv1.UnimplementedComponentDiscoveryServer
	pluginv1.UnimplementedComponentValidationServer
	pluginv1.UnimplementedComponentPreparationServer
	pluginv1.UnimplementedComponentStateServer
	evaluate Evaluator
}

// New constructs a provider with the pinned evaluator when none is supplied.
func New(evaluate Evaluator) *Provider {
	if evaluate == nil {
		evaluate = engine.Resolve
	}
	return &Provider{evaluate: evaluate}
}

func encode(value any) (*pluginv1.ComponentValue, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, component.ErrInvalidValue
	}
	typ, err := ctyjson.ImpliedType(data)
	if err != nil {
		return nil, component.ErrInvalidValue
	}
	v, err := ctyjson.Unmarshal(data, typ)
	if err != nil {
		return nil, component.ErrInvalidValue
	}
	return component.Encode(v)
}

func decode(encoded *pluginv1.ComponentValue, target any) error {
	value, err := component.Decode(encoded)
	if err != nil || !value.IsWhollyKnown() || value.IsNull() {
		return component.ErrInvalidValue
	}
	plain, _ := value.UnmarkDeep()
	data, err := ctyjson.Marshal(plain, plain.Type())
	if err != nil {
		return component.ErrInvalidValue
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return component.ErrInvalidValue
	}
	return nil
}

func settings(value *pluginv1.ComponentValue) error {
	var config map[string]any
	if err := decode(value, &config); err != nil {
		return err
	}
	for key := range config {
		switch key {
		case "enabled", "provider", "mode", "discovery":
		default:
			return ErrUnsupported
		}
	}
	if mode, _ := config["mode"].(string); mode != "" && mode != "native" {
		return ErrUnsupported
	}
	return nil
}

func operation(root, name string, op *pluginv1.ComponentOperation, invocation *pluginv1.Invocation) (model.Request, error) {
	if name != "terragrunt" || !filepath.IsAbs(root) || op == nil || op.Mode != "native" || len(op.Options) != 0 {
		return model.Request{}, ErrUnsupported
	}
	env := maps.Clone(invocation.GetEnvironment())
	if env == nil {
		env = map[string]string{}
	}
	binary := env["ATMOS_TERRAFORM_BINARY"]
	if binary == "" {
		binary = "terraform"
	}
	return model.Request{BasePath: root, Mode: "native", Command: op.Command, Tofu: binary, Environment: env}, nil
}

func engineName(binary string) string {
	if strings.TrimSuffix(filepath.Base(binary), ".exe") == "tofu" {
		return "opentofu"
	}
	return "terraform"
}

func relative(root, path string) (string, error) {
	// Preserve one identity on platforms where temporary directories use symlinks.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", ErrUnsupported
	}
	return filepath.ToSlash(rel), nil
}

func sources(root string, resolved *model.Resolution) ([]*pluginv1.ComponentSource, error) {
	result := []*pluginv1.ComponentSource{}
	seen := map[string]bool{}
	for _, group := range []struct {
		paths []string
		kind  pluginv1.ComponentSource_Kind
	}{{resolved.Files, pluginv1.ComponentSource_FILE}, {resolved.Folders, pluginv1.ComponentSource_DIRECTORY}} {
		for _, path := range group.paths {
			rel, err := relative(root, path)
			if err != nil {
				return nil, err
			}
			key := group.kind.String() + ":" + rel
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, &pluginv1.ComponentSource{Path: rel, Kind: group.kind, Reason: "Terragrunt configuration"})
		}
	}
	return result, nil
}

func rpcError(err error) error {
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "Terragrunt evaluation canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Terragrunt evaluation timed out")
	}
	// HCL diagnostics can contain secrets. Do not copy evaluator payloads into RPC errors.
	if errors.Is(err, ErrUnsupported) {
		return status.Error(codes.Unimplemented, ErrUnsupported.Error())
	}
	return status.Error(codes.InvalidArgument, "Terragrunt evaluation failed")
}

// Validate rejects unsupported operation and provider settings without evaluating HCL.
func (p *Provider) Validate(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, rpcError(err)
	}
	if _, err := operation(request.GetRootDirectory(), request.GetProvider(), request.GetOperation(), request.GetInvocation()); err != nil {
		return nil, rpcError(err)
	}
	if err := settings(request.GetProviderConfiguration()); err != nil {
		return nil, rpcError(err)
	}
	return &pluginv1.ValidateResponse{}, nil
}
