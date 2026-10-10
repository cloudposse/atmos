package starlark

import (
	"context"
	"encoding/json"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

// componentContext installs ctx. A hook's component is a pre-resolved snapshot and is built
// eagerly; a command's component is lazy so scripts that never touch it never resolve it.
func (s *session) componentContext(thread *starlark.Thread) error {
	var selected starlark.Value = starlark.None
	switch {
	case s.spec.Hook != nil && s.spec.Hook.Component != nil:
		var err error
		if selected, err = s.componentValue(thread, s.spec.Hook.Component); err != nil {
			return err
		}
	case s.spec.Component != nil:
		selected = &lazyComponent{s: s, ref: *s.spec.Component}
	}
	hook, operation := hookValues(s.spec.Hook)
	args, file := fileContext(s.spec.File, s.spec.Args)
	flags, err := convert.Dictionary(s.spec.Flags)
	if err != nil {
		return err
	}
	arguments, err := convert.Dictionary(s.spec.Arguments)
	if err != nil {
		return err
	}
	s.globals["ctx"] = starlarkstruct.FromStringDict(starlark.String("ctx"), starlark.StringDict{
		"component": selected, "hook": hook, "operation": operation,
		"args": args, "script": file,
		"flags": flags, "arguments": arguments,
	})
	return nil
}

func (s *session) getComponent(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref script.ComponentRef
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &ref.Name, "stack", &ref.Stack, "type", &ref.Type); err != nil {
		return nil, err
	}
	return s.component(threadContext(thread), ref)
}

// componentEntry memoizes one resolved component. Only successes are cached so a transient
// resolver failure can be retried. Waiting callers can cancel independently of the active resolver.
type componentEntry struct {
	access chan struct{}
	value  starlark.Value
}

// component resolves ref at most once per session; parallel tasks share the result.
func (s *session) component(ctx context.Context, ref script.ComponentRef) (starlark.Value, error) {
	if ref.Name == "" || ref.Stack == "" || ref.Type == "" {
		return nil, invalidArg("component name, stack, and type are required")
	}
	s.componentMu.Lock()
	entry, ok := s.components[ref]
	if !ok {
		entry = &componentEntry{access: make(chan struct{}, 1)}
		s.components[ref] = entry
	}
	s.componentMu.Unlock()

	select {
	case entry.access <- struct{}{}:
		defer func() { <-entry.access }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if entry.value != nil {
		return entry.value, nil
	}
	value, err := s.resolveComponent(ctx, ref)
	if err != nil {
		return nil, err
	}
	entry.value = value
	return value, nil
}

func (s *session) resolveComponent(ctx context.Context, ref script.ComponentRef) (starlark.Value, error) {
	if s.spec.ResolveComponent == nil {
		return nil, fail(errUtils.ErrStarlark, "component resolution is unavailable in this execution context")
	}
	c, err := s.spec.ResolveComponent(ctx, ref)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fail(errUtils.ErrStarlark, "component %q was not resolved", ref.Name)
	}
	value, err := s.componentValue(&starlark.Thread{Name: "component"}, c)
	if err != nil {
		return nil, err
	}
	value.Freeze()
	return value, nil
}

func (s *session) componentValue(thread *starlark.Thread, c *script.Component) (starlark.Value, error) {
	if c.Config == nil {
		return nil, fail(errUtils.ErrStarlark, "component %q has no configuration", c.Name)
	}
	members, err := componentMembers(thread, c)
	if err != nil {
		return nil, err
	}
	base := envpkg.MergeGlobalEnv(s.spec.ProcessEnv, c.Env)
	if s.spec.Hook != nil {
		base = envpkg.MergeGlobalEnv(base, s.spec.Hook.ProcessOverrides)
	}
	base = envpkg.MergeGlobalEnv(base, s.spec.ProcessOverrides)
	base = envpkg.MergeGlobalEnv(base, s.spec.Env)
	dir := c.Path
	members["exec"] = starlark.NewBuiltin("component.exec", func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return s.execScoped(t, b, args, kwargs, processScope{dir: dir, env: base})
	})
	return &componentHandle{
		attrs: starlarkstruct.FromStringDict(starlark.String("component"), members),
		ref:   c.ComponentRef,
	}, nil
}

func componentMembers(thread *starlark.Thread, c *script.Component) (starlark.StringDict, error) {
	encoded, err := json.Marshal(c.Config)
	if err != nil {
		return nil, err
	}
	config, err := starlark.Call(thread, starjson.Module.Members["decode"], starlark.Tuple{starlark.String(encoded)}, nil)
	if err != nil {
		return nil, err
	}
	config.Freeze()
	members := starlark.StringDict{
		"name": starlark.String(c.Name), "stack": starlark.String(c.Stack), "type": starlark.String(c.Type),
		"implementation": starlark.String(c.Implementation), "path": starlark.String(c.Path),
		"config": config, "env": stringDict(c.Env),
	}
	for _, key := range []string{"vars", "settings", "metadata"} {
		value, found, err := config.(*starlark.Dict).Get(starlark.String(key))
		if err != nil {
			return nil, err
		}
		if !found {
			value = starlark.NewDict(0)
			value.Freeze()
		}
		members[key] = value
	}
	return members, nil
}
