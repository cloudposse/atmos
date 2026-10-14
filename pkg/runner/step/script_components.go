package step

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// Serialize stack resolution, which may access shared auth and config caches.
// Component process execution happens outside this lock and remains concurrent.
var scriptResolutionGate = make(chan struct{}, 1)

// SetScriptComponentInfoResolver installs execution-aware component resolution.
func (v *Variables) SetScriptComponentInfoResolver(resolver ComponentInfoResolver) {
	defer perf.Track(nil, "step.Variables.SetScriptComponentInfoResolver")()

	v.scriptComponentInfo = resolver
}

// ScriptComponentRef returns the component the step variables are scoped to, or nil
// when the execution context has none. Control-step adapters use it to give
// parallel/matrix children the same `ctx.component` the parent step would see.
func ScriptComponentRef(vars *Variables) *script.ComponentRef {
	defer perf.Track(nil, "step.ScriptComponentRef")()

	if vars == nil {
		return nil
	}
	config, ok := vars.templateRoots["Component"].(map[string]any)
	if !ok {
		return nil
	}
	ref := &script.ComponentRef{}
	ref.Name, _ = config["atmos_component"].(string)
	ref.Stack, _ = config["atmos_stack"].(string)
	ref.Type, _ = config["component_type"].(string)
	if ref.Name == "" || ref.Stack == "" || ref.Type == "" {
		return nil
	}
	return ref
}

// ScriptComponentResolver returns the execution-aware component resolver backing
// `components.get` for embedded scripts. The resolver reports an ErrScript error
// when the variables carry no resolver, so it is always safe to install.
func ScriptComponentResolver(vars *Variables) script.ComponentResolver {
	defer perf.Track(nil, "step.ScriptComponentResolver")()

	if vars == nil {
		return nil
	}
	return func(ctx context.Context, ref script.ComponentRef) (*script.Component, error) {
		if vars.scriptComponentInfo == nil || vars.AtmosConfig == nil {
			return nil, fmt.Errorf("%w: component resolution is unavailable in this execution context", errUtils.ErrScript)
		}
		select {
		case scriptResolutionGate <- struct{}{}:
			defer func() { <-scriptResolutionGate }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := vars.scriptComponentInfo(ctx, ref.Name, ref.Stack, ref.Type)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return scriptComponent(vars.AtmosConfig, ref, info)
	}
}

func scriptComponent(config *schema.AtmosConfiguration, ref script.ComponentRef, info *schema.ConfigAndStacksInfo) (*script.Component, error) {
	if info == nil || info.ComponentSection == nil {
		return nil, fmt.Errorf("%w: component %q was not resolved", errUtils.ErrScript, ref.Name)
	}
	implementation := scriptImplementation(info.ComponentSection, ref.Name)
	path, err := scriptComponentPath(config, ref.Type, implementation, info.ComponentSection)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(info.ComponentSection)
	if err != nil {
		return nil, err
	}
	var snapshot map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, err
	}
	env := map[string]string{}
	for key, value := range info.ComponentEnvSection {
		if value != nil && value != "null" {
			env[key] = fmt.Sprint(value)
		}
	}
	return &script.Component{ComponentRef: ref, Implementation: implementation, Path: path, Config: snapshot, Env: env}, nil
}

func scriptImplementation(section map[string]any, fallback string) string {
	if metadata, ok := section["metadata"].(map[string]any); ok {
		if implementation, ok := metadata["component"].(string); ok && implementation != "" {
			return implementation
		}
	}
	if implementation, ok := section["component"].(string); ok && implementation != "" {
		return implementation
	}
	return fallback
}

func scriptComponentPath(config *schema.AtmosConfiguration, componentType, implementation string, section map[string]any) (string, error) {
	base := filepath.Join("components", componentType)
	if provider, ok := component.GetProvider(componentType); ok {
		base = provider.GetBasePath(config)
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(config.BasePath, base)
	}
	path, err := filepath.Abs(filepath.Join(base, implementation))
	if err != nil {
		return "", err
	}
	if details, ok := section["component_info"].(map[string]any); ok {
		if resolved, ok := details["component_path"].(string); ok && resolved != "" {
			path = resolved
		}
	}
	return path, nil
}
