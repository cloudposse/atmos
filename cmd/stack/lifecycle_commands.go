package stack

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

func discoverLifecycleCommands(root *cobra.Command, config *schema.AtmosConfiguration, verb string) map[string]*lifecycle.Handler {
	commands := make(map[string]*lifecycle.Handler)
	for kind, handler := range lifecycle.DefaultRegistry.Handlers(lifecycle.Event(verb)) {
		_, rest, err := root.Find(handler.Path)
		if err == nil && len(rest) == 0 {
			commands[kind] = handler
		}
	}
	discovery := lifecycleDiscovery{root: root, verb: verb, result: commands}
	discovery.walk(config.Commands, nil, nil)
	return commands
}

type lifecycleDiscovery struct {
	root   *cobra.Command
	verb   string
	result map[string]*lifecycle.Handler
}

func (d *lifecycleDiscovery) walk(configs []schema.Command, parent []string, inherited []schema.CommandFlag) {
	for i := range configs {
		c := &configs[i]
		path := append(slices.Clone(parent), c.Name)
		flags := mergeLifecycleFlags(inherited, c.Flags)
		if c.Name == d.verb && c.Component != nil && c.Component.Type != "" {
			found, rest, err := d.root.Find(path)
			// A custom declaration shadowed by a built-in must not change its
			// capability binding. The existing command collision policy wins.
			if err == nil && len(rest) == 0 && found.Annotations["customCommand"] == "true" {
				addLifecycleCommand(d.result, c, path, flags)
			}
		}
		d.walk(c.Commands, path, flags)
	}
}

func addLifecycleCommand(result map[string]*lifecycle.Handler, c *schema.Command, path []string, flags []schema.CommandFlag) {
	kind := c.Component.Type
	command := &lifecycle.Handler{Path: path, Args: c.Arguments, Flags: flags}
	if previous, exists := result[kind]; exists {
		command.Err = fmt.Errorf("%w: ambiguous %s operation: %s and %s", errUtils.ErrInvalidConfig, kind, strings.Join(previous.Path, " "), strings.Join(path, " "))
	}
	if err := c.Steps.LoadError(); err != nil {
		command.Err = err
	}
	result[kind] = command
}

func mergeLifecycleFlags(parent, child []schema.CommandFlag) []schema.CommandFlag {
	result := slices.Clone(parent)
	for i := range child {
		flag := &child[i]
		idx := slices.IndexFunc(result, func(f schema.CommandFlag) bool { return f.Name == flag.Name })
		if idx < 0 {
			result = append(result, *flag)
		} else {
			result[idx] = *flag
		}
	}
	return result
}
