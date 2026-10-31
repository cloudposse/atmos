// Package execution applies provider-evaluated hooks and extra arguments to Terraform commands.
//
// A configuration provider (for example the native Terragrunt plugin) evaluates hooks and extra
// arguments but never runs them. The host owns execution: it matches them against the command
// being run, runs hooks with the invocation's environment, working directory, cancellation, and
// I/O, and surfaces failures with their exit codes. The rules follow Terragrunt's own: a command
// is matched by name, `apply -destroy` counts as `destroy`, before hooks gate the command, after
// hooks need run_on_error to follow a failure, and error hooks match on the failure text.
package execution

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Lifecycle commands name stages of a provider's own pipeline instead of Terraform commands. A hook
// lists them in `commands` like any command, and they run in the unit's configuration directory.
const (
	// CommandReadConfig runs after_hooks once the unit's configuration is read.
	CommandReadConfig = "terragrunt-read-config"
	// CommandReadConfigAlias is the documented spelling of CommandReadConfig.
	CommandReadConfigAlias = "read-config"
	// CommandInitFromModule runs hooks around the module source being fetched.
	CommandInitFromModule = "init-from-module"
)

const (
	commandApply   = "apply"
	commandDestroy = "destroy"
	destroyFlag    = "-destroy"
)

// HookKind names the block a hook was declared in.
type HookKind string

// Hook kinds in the order their lifecycle runs.
const (
	KindBefore HookKind = "before_hook"
	KindAfter  HookKind = "after_hook"
	KindError  HookKind = "error_hook"
)

// CommandName returns the name that hooks and extra arguments match against.
// It is "destroy" for `destroy` and for `apply` with the destroy flag set, and the Terraform
// command itself otherwise, so `plan -destroy` still matches "plan". Like Terragrunt, only the
// command is matched, never its subcommand: `state list` and `state rm` both match "state".
func CommandName(command string, flags []string) string {
	defer perf.Track(nil, "execution.CommandName")()

	if fields := strings.Fields(command); len(fields) > 0 {
		command = fields[0]
	}
	if command == commandDestroy || (command == commandApply && hasDestroyFlag(flags)) {
		return commandDestroy
	}
	return command
}

// hasDestroyFlag reads the flag as Terraform does: one or two dashes, an optional boolean
// value, and the last occurrence wins.
func hasDestroyFlag(flags []string) bool {
	destroy := false
	for _, flag := range flags {
		name, raw, hasValue := strings.Cut(flag, "=")
		if name != destroyFlag && name != "-"+destroyFlag {
			continue
		}
		if !hasValue {
			destroy = true
			continue
		}
		parsed, err := strconv.ParseBool(raw)
		destroy = err == nil && parsed
	}
	return destroy
}

// Validate rejects hooks the host cannot run, so a malformed provider response fails before any
// command starts instead of silently skipping a hook.
func Validate(config *schema.ComponentExecution) error {
	defer perf.Track(nil, "execution.Validate")()

	if config == nil {
		return nil
	}
	for _, group := range []struct {
		kind  HookKind
		hooks []schema.ComponentHook
	}{{KindBefore, config.BeforeHooks}, {KindAfter, config.AfterHooks}, {KindError, config.ErrorHooks}} {
		for i := range group.hooks {
			if err := validateHook(group.kind, &group.hooks[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateHook(kind HookKind, hook *schema.ComponentHook) error {
	if len(hook.Execute) == 0 || strings.TrimSpace(hook.Execute[0]) == "" {
		return fmt.Errorf("%w: %s %q has no command to execute", errUtils.ErrPluginProtocol, kind, hook.Name)
	}
	for _, pattern := range hook.OnErrors {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%w: %s %q on_errors is not a valid regular expression", errUtils.ErrPluginProtocol, kind, hook.Name)
		}
	}
	return nil
}

// hooksFor keeps declaration order and selects the hooks that list the command.
func hooksFor(hooks []schema.ComponentHook, command string) []*schema.ComponentHook {
	var selected []*schema.ComponentHook
	for i := range hooks {
		for _, listed := range hooks[i].Commands {
			if listed == command {
				selected = append(selected, &hooks[i])
				break
			}
		}
	}
	return selected
}

// ListsCommand reports whether any hook lists one of the commands.
func ListsCommand(hooks []schema.ComponentHook, commands ...string) bool {
	defer perf.Track(nil, "execution.ListsCommand")()

	for i := range hooks {
		for _, listed := range hooks[i].Commands {
			if slices.Contains(commands, listed) {
				return true
			}
		}
	}
	return false
}

// HooksFor returns copies of the hooks that list any of the commands, each rewritten to list only
// the canonical command, so a hook written with an alias still matches the phase that runs it.
func HooksFor(hooks []schema.ComponentHook, canonical string, aliases ...string) []schema.ComponentHook {
	defer perf.Track(nil, "execution.HooksFor")()

	var selected []schema.ComponentHook
	for i := range hooks {
		if !ListsCommand(hooks[i:i+1], append([]string{canonical}, aliases...)...) {
			continue
		}
		hook := hooks[i]
		hook.Commands = []string{canonical}
		selected = append(selected, hook)
	}
	return selected
}

// ExcludeMatches reports whether an exclude block's actions name the command. "all" matches every
// command and "all_except_output" every command but output; other entries match case-insensitively.
func ExcludeMatches(actions []string, command string) bool {
	defer perf.Track(nil, "execution.ExcludeMatches")()

	command = strings.ToLower(command)
	for _, action := range actions {
		switch {
		case action == "all":
			return true
		case action == "all_except_output" && command != "output":
			return true
		case strings.ToLower(action) == command:
			return true
		}
	}
	return false
}
