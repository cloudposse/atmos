package hooks

import (
	"fmt"
	"maps"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// scriptComponentResolver returns the resolver that backs `components.get` for a hook's
// embedded script steps. The hook's own `ctx.component` is a pre-resolved snapshot; this
// resolver serves lookups of other components. An injected ExecContext.ComponentResolver wins;
// otherwise the execution pipeline is used with the operation's auth manager when it has one.
func scriptComponentResolver(ctx *ExecContext) runnerstep.ComponentInfoResolver {
	if ctx.ComponentResolver != nil {
		return ctx.ComponentResolver
	}
	var manager auth.AuthManager
	if ctx.Info != nil {
		manager, _ = ctx.Info.AuthManager.(auth.AuthManager)
	}
	return e.ScriptComponentInfoResolver(ctx.AtmosConfig, manager)
}

// scriptHookContext uses the execution snapshot rather than re-resolving the
// stack, which could fetch secrets again or choose a different component workdir.
func scriptHookContext(ctx *ExecContext) *script.HookContext {
	event := ctx.Event.Normalize()
	parts := strings.SplitN(string(event), ".", 2)
	command := string(event)
	if len(parts) == 2 {
		command = strings.ReplaceAll(strings.TrimSuffix(parts[1], ".aggregate"), ".", " ")
	}
	hook := &script.HookContext{Name: ctx.HookName, Event: string(event), Operation: script.Operation{Command: command}}
	// RunAll defaults the outcome to success even before execution, for hook
	// filtering. That default is not an actual operation result.
	if event.IsPostExecution() && ctx.Outcome.Status != "" {
		status, code := string(ctx.Outcome.Status), ctx.Outcome.ExitCode
		hook.Operation.Status, hook.Operation.ExitCode = &status, &code
		if ctx.Outcome.Err != nil {
			message := ctx.Outcome.Err.Error()
			hook.Operation.Error = &message
		}
	}
	hook.Component = scriptHookComponent(ctx)
	if ctx.Hook != nil {
		hook.ProcessOverrides = maps.Clone(ctx.Hook.Env)
	}
	return hook
}

func scriptHookComponent(ctx *ExecContext) *script.Component {
	info := ctx.Info
	if ctx.Hook != nil && ctx.Hook.stepTemplateInfo != nil {
		info = ctx.Hook.stepTemplateInfo
	}
	if info == nil || info.ComponentFromArg == "" || info.Stack == "" || strings.HasSuffix(string(ctx.Event), ".aggregate") {
		return nil
	}
	section := hookComponentSection(info)
	componentType := info.ComponentType
	if componentType == "" {
		parts := strings.Split(string(ctx.Event), ".")
		if len(parts) > 1 {
			componentType = parts[1]
		}
	}
	return &script.Component{
		ComponentRef:   script.ComponentRef{Name: info.ComponentFromArg, Stack: info.Stack, Type: componentType},
		Implementation: hookComponentImplementation(section, info.FinalComponent, info.ComponentFromArg),
		Path:           ComponentPath(ctx), Config: section, Env: hookComponentEnv(section),
	}
}

func hookComponentSection(info *schema.ConfigAndStacksInfo) map[string]any {
	section := maps.Clone(info.ComponentSection)
	if section == nil {
		section = make(map[string]any)
	}
	// Dedicated execution sections can be newer than the captured template data.
	for key, value := range map[string]map[string]any{
		"vars": info.ComponentVarsSection, "settings": info.ComponentSettingsSection, "env": info.ComponentEnvSection,
	} {
		if value != nil {
			section[key] = value
		}
	}
	return section
}

func hookComponentImplementation(section map[string]any, implementation, fallback string) string {
	if implementation == "" {
		implementation, _ = section["component"].(string)
	}
	if metadata, ok := section["metadata"].(map[string]any); ok {
		if name, ok := metadata["component"].(string); ok && name != "" {
			implementation = name
		}
	}
	if implementation == "" {
		implementation = fallback
	}
	return implementation
}

func hookComponentEnv(section map[string]any) map[string]string {
	env := make(map[string]string)
	if values, ok := section["env"].(map[string]any); ok {
		for key, value := range values {
			if value != nil && value != "null" {
				env[key] = fmt.Sprint(value)
			}
		}
	}
	return env
}
