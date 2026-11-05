package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	semanticComponent = "component"
	semanticStack     = "stack"
)

// Handler binds a component kind's lifecycle event to an existing command. Invalid
// declarations retain their error so an unrelated kind does not fail selection.
type Handler struct {
	Path  []string
	Args  []schema.CommandArgument
	Flags []schema.CommandFlag
	Err   error
	// HookEvents are the actual per-component hook events fired by this command.
	HookEvents []string
	// ApprovalArgs opt into the component's own non-interactive approval behavior.
	ApprovalArgs []string
	// NewResourceGuard creates one shared lock domain for this invocation.
	NewResourceGuard func(*schema.AtmosConfiguration) func(*dependency.Node) func()
	// WorkerEnvironment supplies execution policy owned by this component type.
	WorkerEnvironment func(int) []string
}

// NativeHandler describes the standard component positional and stack flag.
func NativeHandler(path ...string) *Handler {
	defer perf.Track(nil, "lifecycle.NativeHandler")()

	return &Handler{
		Path:  path,
		Args:  []schema.CommandArgument{{Name: semanticComponent, Provides: semanticComponent, Required: true}},
		Flags: []schema.CommandFlag{{Name: semanticStack, Provides: semanticStack, Required: true}},
	}
}

// Arguments resolves a concrete invocation without shell interpolation.
func (c *Handler) Arguments(node *dependency.Node) ([]string, error) {
	defer perf.Track(nil, "lifecycle.Handler.Arguments")()
	if c == nil || node == nil {
		return nil, errUtils.ErrNilParam
	}
	if c.Err != nil {
		return nil, c.Err
	}
	bindings := map[string]int{}
	flags, err := c.flagArguments(node, bindings)
	if err != nil {
		return nil, err
	}
	positional, err := c.positionalArguments(node, bindings)
	if err != nil {
		return nil, err
	}
	if bindings[semanticComponent] != 1 || bindings[semanticStack] != 1 {
		return nil, c.invalid("requires exactly one component binding and one stack binding")
	}
	args := append(slices.Clone(c.Path), flags...)
	return append(args, positional...), nil
}

func (c *Handler) flagArguments(node *dependency.Node, bindings map[string]int) ([]string, error) {
	var args []string
	for i := range c.Flags {
		flag := &c.Flags[i]
		semantic := flag.EffectiveProvides()
		if semantic == "" {
			if flag.Required && flag.Default == nil {
				return nil, c.invalid("required flag --" + flag.Name + " has no default")
			}
			if flag.Required {
				args = append(args, "--"+flag.Name+"="+fmt.Sprint(flag.Default))
			}
			continue
		}
		if flag.Type != "" && flag.Type != "string" {
			return nil, c.invalid("semantic flag --" + flag.Name + " must be a string")
		}
		value, err := c.resolve(semantic, node, bindings, flag.Values)
		if err != nil {
			return nil, err
		}
		args = append(args, "--"+flag.Name+"="+value)
	}
	return args, nil
}

func (c *Handler) positionalArguments(node *dependency.Node, bindings map[string]int) ([]string, error) {
	var positional []string
	for i := range c.Args {
		arg := &c.Args[i]
		value := arg.Default
		if semantic := arg.EffectiveProvides(); semantic != "" {
			var err error
			value, err = c.resolve(semantic, node, bindings, arg.Values)
			if err != nil {
				return nil, err
			}
		} else if arg.Required && value == "" {
			return nil, c.invalid("required argument " + arg.Name + " has no default")
		}
		// Atmos reserves -- for native passthrough, even on custom commands.
		if strings.HasPrefix(value, "-") {
			return nil, c.invalid("positional argument " + arg.Name + " starts with '-'; use a semantic flag binding")
		}
		positional = append(positional, value)
	}
	for len(positional) > 0 && positional[len(positional)-1] == "" {
		positional = positional[:len(positional)-1]
	}
	return positional, nil
}

func (c *Handler) resolve(semantic string, node *dependency.Node, bindings map[string]int, allowed []string) (string, error) {
	bindings[semantic]++
	var value string
	switch semantic {
	case semanticComponent:
		value = node.Component
	case semanticStack:
		value = node.Stack
	default:
		return "", c.invalid("unsupported semantic input " + semantic)
	}
	if len(allowed) > 0 && !slices.Contains(allowed, value) {
		return "", c.invalid(fmt.Sprintf("%s %q is not an allowed value", semantic, value))
	}
	return value, nil
}

func (c *Handler) invalid(reason string) error {
	return fmt.Errorf("%w: %s: %s", errUtils.ErrInvalidConfig, strings.Join(c.Path, " "), reason)
}
