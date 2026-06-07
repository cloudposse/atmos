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

	if err := guardInstanceOverride(svc, scope, target.name); err != nil {
		return err
	}

	if err := svc.Set(target.name, resolvedValue); err != nil {
		return err
	}

	ui.Successf("Set secret `%s` for component `%s` in stack `%s`", target.name, scope.Component, scope.Stack)
	return nil
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

// guardInstanceOverride rejects setting an instance-level value for a secret that is stack-scoped
// at the targeted component. An instance may only carry its own value if it declares the secret
// under the component (which pulls it to instance scope); otherwise the write would silently shadow
// the shared stack value, so it is a hard error. Secrets that are already instance-scoped, or whose
// scope can't be resolved (let the backend decide), pass through.
func guardInstanceOverride(svc secretService, scope secretScope, name string) error {
	if scope.Component == "" {
		return nil
	}
	if sc, ok := svc.ScopeOf(name); ok && sc == secrets.ScopeStack {
		return errUtils.Build(secrets.ErrSecretNotOverridable).
			WithExplanationf("secret %q is stack-scoped in stack %q; component %q has not declared it as an instance override", name, scope.Stack, scope.Component).
			WithHintf("declare %q under component %q's `secrets.vars` to override it at instance scope, or omit --component to set the shared stack value", name, scope.Component).
			Err()
	}
	return nil
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
