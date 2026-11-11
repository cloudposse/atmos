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

	"github.com/zclconf/go-cty/cty"
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
func New(evaluate Evaluator) *Provider {
	if evaluate == nil {
		evaluate = engine.Resolve
	}
	return &Provider{evaluate: evaluate}
}

func encode(value any) (*pluginv1.ComponentValue, error) {
	return encodeSensitive(value, nil)
}

// encodeSensitive encodes value with the given paths marked sensitive. A path is a list of object keys; the
// empty path marks the whole value. Numbers keep their exact decimal digits because the JSON form is
// read with cty's arbitrary-precision numbers, and null stays null.
func encodeSensitive(value any, sensitive [][]string) (*pluginv1.ComponentValue, error) {
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
	return component.Encode(markSensitive(v, sensitive))
}

// markSensitive marks the values at the given object-key paths sensitive. A path that does not lead to a
// value is skipped: the scan that produced it may be conservative.
func markSensitive(value cty.Value, paths [][]string) cty.Value {
	var marks []cty.PathValueMarks
	for _, steps := range paths {
		if len(steps) == 0 {
			return value.Mark(component.Sensitive)
		}
		path := cty.Path{}
		for _, step := range steps {
			path = path.GetAttr(step)
		}
		if _, err := path.Apply(value); err != nil {
			continue
		}
		marks = append(marks, cty.PathValueMarks{Path: path, Marks: cty.NewValueMarks(component.Sensitive)})
	}
	if len(marks) == 0 {
		return value
	}
	return value.MarkWithPaths(marks)
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
	if mode, _ := config["mode"].(string); mode != "" && mode != ModeNative && mode != ModeExecutable {
		return settingProblem("mode", "Unsupported value for `components.terraform.terragrunt.mode`", "The supported modes are `native` and `executable`. Remove `mode` or set one of them.")
	}
	return nil
}

// Modes a section can run in. Native adapts units to Atmos; executable runs them with the terragrunt
// executable, so the plugin only describes units and never prepares them.
const (
	ModeNative     = "native"
	ModeExecutable = "executable"
)

// toolEnvironment maps the options that Terragrunt reads from the environment to their variables.
// An option replaces the variable, so it ranks above the environment and the unit, which is the
// order of Terragrunt's own flags.
var toolEnvironment = map[string]string{
	"iam_assume_role":                    "TG_IAM_ASSUME_ROLE",
	"iam_assume_role_duration":           "TG_IAM_ASSUME_ROLE_DURATION",
	"iam_assume_role_session_name":       "TG_IAM_ASSUME_ROLE_SESSION_NAME",
	"iam_assume_role_web_identity_token": "TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN",
	"download_dir":                       "TG_DOWNLOAD_DIR",
}

// applyOptions validates the typed provider options of the operation, overrides the environment they
// replace, and returns the feature overrides. An unknown option is rejected rather than ignored, so a
// host and a plugin that disagree about the declared options fail loudly.
func applyOptions(env map[string]string, options map[string]*pluginv1.ComponentValue) (map[string]string, error) {
	var features map[string]string
	for _, name := range slices.Sorted(maps.Keys(options)) {
		if name == "feature" {
			if err := decode(options[name], &features); err != nil {
				return nil, optionProblem(name, "Use name=value pairs such as `--terragrunt-feature alpha=true`.")
			}
			continue
		}
		variable, known := toolEnvironment[name]
		if !known {
			return nil, optionProblem(name, "Check the option name against the package's declared options.")
		}
		var text string
		if name == "iam_assume_role_duration" {
			var value int
			if err := decode(options[name], &value); err != nil {
				return nil, optionProblem(name, "The value has an unexpected type.")
			}
			text = fmt.Sprint(value)
		} else if err := decode(options[name], &text); err != nil {
			return nil, optionProblem(name, "The value has an unexpected type.")
		}
		env[variable] = text
	}
	return features, nil
}

func optionProblem(name, hint string) *model.Problem {
	return &model.Problem{Code: model.CodeUnsupportedSetting, Summary: fmt.Sprintf("Invalid provider option `%s`", name), Hints: []string{hint}, Key: "options." + name, Safe: true, Err: ErrUnsupported}
}

func operation(root, name string, op *pluginv1.ComponentOperation, invocation *pluginv1.Invocation) (model.Request, error) {
	if name != "terragrunt" || !filepath.IsAbs(root) || op == nil || (op.Mode != ModeNative && op.Mode != ModeExecutable) {
		return model.Request{}, ErrUnsupported
	}
	env := maps.Clone(invocation.GetEnvironment())
	if env == nil {
		env = map[string]string{}
	}
	features, err := applyOptions(env, op.Options)
	if err != nil {
		return model.Request{}, err
	}
	binary := env["ATMOS_TERRAFORM_BINARY"]
	if binary == "" {
		binary = "terraform"
	}
	return model.Request{BasePath: root, Mode: ModeNative, Command: op.Command, Arguments: slices.Clone(op.TerraformArguments), Tofu: binary, Environment: env, Features: features}, nil
}

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
func (p *Provider) Validate(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
	response, err := p.validate(ctx, request)
	return response, scoped(err, "validate", request.GetRootDirectory(), request.GetAddress())
}

func (p *Provider) validate(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
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
