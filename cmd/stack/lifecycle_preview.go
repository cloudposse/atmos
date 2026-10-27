package stack

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/ui"
)

func previewLifecycle(plan *lifecycle.Plan, verb string, globalArgs []string, cmd *cobra.Command) error {
	order, err := plan.Graph.TopologicalSort()
	if err != nil {
		return err
	}
	if len(order) == 0 {
		ui.Info("No components matched")
		return nil
	}
	for i := range order {
		n := &order[i]
		ui.Infof("Would %s %s", verb, lifecycle.Label(n))
		args := append(append([]string{}, globalArgs...), plan.Commands[n.ID]...)
		ui.Writef("  command: atmos %s\n", shellescape.QuoteCommand(args))
		var waits []string
		for _, id := range n.Dependencies {
			waits = append(waits, lifecycle.Label(plan.Graph.Nodes[id]))
		}
		sort.Strings(waits)
		if len(waits) > 0 {
			ui.Writef("  waits for: %s\n", strings.Join(waits, ", "))
		}
		var events []string
		if handler := plan.Handlers[n.Type]; handler != nil {
			events = handler.HookEvents
		}
		previews, err := previewHooks(n, events, hooks.ResolveSkipHooks(cmd))
		if err != nil {
			return err
		}
		for _, preview := range previews {
			ui.Writef("  %s\n", preview)
		}
	}
	return nil
}

// previewHooks decodes static metadata and uses the real event matcher, but never
// evaluates conditions, templates, tool availability, or a hook engine. Hook names
// are sorted for readable output; ordering within an event belongs to the engine.
func previewHooks(n *dependency.Node, events []string, skip string) ([]string, error) {
	section := n.Metadata["hooks"]
	if section == nil {
		return nil, nil
	}
	raw, err := yaml.Marshal(section)
	if err != nil {
		return nil, err
	}
	var items map[string]hooks.Hook
	if err := yaml.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%s hooks: %w", lifecycle.Label(n), err)
	}
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	var result []string
	skipHook := hooks.NewSkipPredicate(skip)
	for _, event := range events {
		for _, name := range names {
			h := items[name]
			if !h.MatchesEvent(hooks.HookEvent(event)) {
				continue
			}
			result = append(result, formatHookPreview(&h, name, event, skipHook(name)))
		}
	}
	return result, nil
}

func formatHookPreview(h *hooks.Hook, name, event string, skipped bool) string {
	condition := "success (default)"
	if !h.When.IsZero() {
		encoded, _ := yaml.Marshal(h.When)
		condition = strings.TrimSpace(string(encoded))
	}
	status := "would run if condition matches; condition evaluated at execution"
	if skipped {
		status = "skipped by --skip-hooks"
	}
	return fmt.Sprintf("hook %s: event=%s kind=%s when=%s action=%s (%s)", name, event, h.Kind, condition, hookPreviewAction(h), status)
}

func hookPreviewAction(h *hooks.Hook) string {
	action := h.Command
	if h.Type != "" {
		action += " type=" + h.Type
	}
	if len(h.Args) > 0 {
		action += " " + shellescape.QuoteCommand(h.Args)
	}
	if h.With != nil {
		encoded, _ := json.Marshal(h.With)
		if len(encoded) == 0 {
			encoded, _ = yaml.Marshal(h.With)
		}
		action += " " + strings.TrimSpace(string(encoded))
	}
	if action != "" {
		return action
	}
	// Store, Git, and migration hooks declare their action in kind-specific fields.
	// Preserve those fields without resolving any values.
	declaration := *h
	declaration.Events = nil
	declaration.When = schema.Condition{}
	encoded, _ := yaml.Marshal(declaration)
	return strings.TrimSpace(string(encoded))
}
