package secret

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

var setParser *flags.StandardParser

var setCmd = &cobra.Command{
	Use:     "set [NAME[=VALUE]]",
	Aliases: []string{"add"},
	Short:   "Set a declared secret's value (create or update).",
	Long: "Set a declared secret's value. Provide the value inline as NAME=VALUE, via --stdin, or " +
		"interactively when running in a terminal. With no NAME on a TTY, Atmos prompts for the " +
		"stack, component, and secret to set.",
	Args: cobra.MaximumNArgs(1),
	RunE: runSecretSet,
}

func init() {
	setParser = flags.NewStandardParser(
		flags.WithBoolFlag("stdin", "", false, "Read the secret value from standard input"),
		flags.WithBoolFlag("force", "f", false, "Overwrite an existing value without confirmation"),
	)
	setParser.RegisterFlags(setCmd)
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretSet")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	target, err := resolveSetName(svc, args)
	if err != nil {
		return err
	}

	useStdin, _ := cmd.Flags().GetBool("stdin")
	resolvedValue, err := resolveSetValue(target.value, target.hasValue, useStdin)
	if err != nil {
		return err
	}

	if err := svc.Set(target.name, resolvedValue); err != nil {
		return err
	}

	ui.Success(setSuccessMessage(svc, scope, target.name))
	return nil
}

// setSuccessMessage describes where the value was written: shared scopes (stack, global) name the
// shared location so the user knows every consumer sees the new value.
func setSuccessMessage(svc secretService, scope secretScope, name string) string {
	sc, _ := svc.ScopeOf(name)
	switch sc {
	case secrets.ScopeGlobal:
		return fmt.Sprintf("Set global secret `%s` (one value shared by every stack and component using its store)", name)
	case secrets.ScopeStack:
		return fmt.Sprintf("Set stack-scoped secret `%s` for stack `%s` (shared by every component in the stack)", name, scope.Stack)
	default:
		return fmt.Sprintf("Set secret `%s` for component `%s` in stack `%s`", name, scope.Component, scope.Stack)
	}
}

// setTarget is the resolved secret name and any inline value parsed for `secret set`.
type setTarget struct {
	name     string
	value    string
	hasValue bool
}

// resolveSetName determines the secret name (and any inline value) to set. With a positional arg it
// parses NAME[=VALUE]; with none it prompts to pick a declared secret for the resolved scope on a
// TTY, and falls back to the standard "NAME required" error in non-interactive contexts.
func resolveSetName(svc secretService, args []string) (setTarget, error) {
	if len(args) > 0 {
		name, value, hasValue := strings.Cut(args[0], "=")
		name = strings.TrimSpace(name)
		if name == "" {
			return setTarget{}, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
				WithExplanation("secret NAME is required").Err()
		}
		return setTarget{name: name, value: value, hasValue: hasValue}, nil
	}

	names := declaredNames(svc)
	chosen, promptErr := flags.PromptForValue("secret", "Choose a secret to set", names)
	if promptErr != nil {
		if errors.Is(promptErr, errUtils.ErrInteractiveModeNotAvailable) || errors.Is(promptErr, errUtils.ErrNoOptionsAvailable) {
			return setTarget{}, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
				WithExplanation("secret NAME is required").
				WithHint("Pass a NAME (e.g. `atmos secret set DATADOG_API_KEY ...`) or run in an interactive terminal to choose one").Err()
		}
		return setTarget{}, promptErr
	}
	return setTarget{name: chosen}, nil
}

// declaredNames returns the sorted declared secret names for the service's scope.
func declaredNames(svc secretService) []string {
	decls := svc.Declarations()
	names := make([]string, 0, len(decls))
	for i := range decls {
		names = append(names, decls[i].Name)
	}
	sort.Strings(names)
	return names
}

// resolveSetValue determines the secret value from the inline value, stdin, or a prompt.
func resolveSetValue(inlineValue string, hasInline, useStdin bool) (string, error) {
	if useStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read secret from stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	if hasInline {
		return inlineValue, nil
	}
	return promptForValueFn()
}
