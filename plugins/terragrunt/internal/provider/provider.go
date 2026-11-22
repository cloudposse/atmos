// Package provider adapts Terragrunt evaluation to Atmos component services.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	ctyjson "github.com/zclconf/go-cty/cty/json"

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
//
//nolint:lintroller // This SDK-only factory cannot import the host performance tracker.
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
	for _, key := range slices.Sorted(maps.Keys(config)) {
		switch key {
		case "enabled", "provider", "mode", "discovery", "features":
		default:
			return settingProblem(key, fmt.Sprintf("Unknown setting `components.terraform.terragrunt.%s`", key), "Valid keys are `enabled`, `provider`, `mode`, `discovery`, and `features`. Check the spelling.")
		}
	}
	if mode, _ := config["mode"].(string); mode != "" && mode != "native" {
		return settingProblem("mode", "Unsupported value for `components.terraform.terragrunt.mode`", "The only supported mode is `native`. Remove `mode` or set `mode: native`.")
	}
	return nil
}

// optionSourceHooksCompleted is the only operation option. The host sets it to true when it prepares
// a unit again after running the unit's read-config after hooks, so that the registry source that the
// first preparation left as written is resolved. The host and the plugin share the name.
const optionSourceHooksCompleted = "source_hooks_completed"

// sourceHooksCompleted reads the operation options. Any other option, or a value that is not a
// boolean, is rejected, so the host never believes an option was honored when it was not.
func sourceHooksCompleted(options map[string]*pluginv1.ComponentValue) (bool, error) {
	completed := false
	for name, value := range options {
		if name != optionSourceHooksCompleted || decode(value, &completed) != nil {
			return false, ErrUnsupported
		}
	}
	return completed, nil
}

func operation(root, name string, op *pluginv1.ComponentOperation, invocation *pluginv1.Invocation) (model.Request, error) {
	if name != "terragrunt" || !filepath.IsAbs(root) || op == nil || (op.Mode != "native" && op.Mode != "iam-probe") {
		return model.Request{}, ErrUnsupported
	}
	completed, err := sourceHooksCompleted(op.Options)
	if err != nil {
		return model.Request{}, err
	}
	env := maps.Clone(invocation.GetEnvironment())
	if env == nil {
		env = map[string]string{}
	}
	binary := env["ATMOS_TERRAFORM_BINARY"]
	if binary == "" {
		binary = "terraform"
	}
	request := model.Request{BasePath: root, Mode: "native", Command: op.Command, Arguments: slices.Clone(op.TerraformArguments), Tofu: binary, Environment: env, SourceHooksCompleted: completed}
	if op.Mode == "iam-probe" {
		request.Phase = "iam"
	}
	return request, nil
}

// engineName names the engine of an executable with the rule the engine package shares with the host.
func engineName(binary string) string {
	if engine.IsOpenTofu(binary) {
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

// Validate rejects unsupported operation and provider settings without evaluating HCL.
//
//nolint:lintroller // Standalone plugin code cannot import the host performance tracker.
func (p *Provider) Validate(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
	if err := validate(ctx, request); err != nil {
		return nil, scoped(rpcError(err), "validate", request.GetRootDirectory(), request.GetAddress())
	}
	return &pluginv1.ValidateResponse{}, nil
}

// validate returns the unconverted cause so callers map it to a status code exactly once.
func validate(ctx context.Context, request *pluginv1.ComponentRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := operation(request.GetRootDirectory(), request.GetProvider(), request.GetOperation(), request.GetInvocation()); err != nil {
		return err
	}
	return settings(request.GetProviderConfiguration())
}
