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

	scope, err := parseSetScope(cmd, args)
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

// parseSetScope permits --component to be omitted only when a positional name resolves to one
// consistent global declaration in the selected stack. A component is still used internally to
// load the inherited declaration, but it cannot affect the resulting global backend coordinate.
func parseSetScope(cmd *cobra.Command, args []string) (secretScope, error) {
	scope, err := parseScopeStack(cmd, args)
	if err != nil {
		return scope, err
	}
	if scope.Component != "" || len(args) == 0 {
		return requireScopeComponent(scope, cmd, args)
	}
	target, err := setTargetFromArg(args[0])
	if err != nil {
		return scope, err
	}
	component, componentType, err := findGlobalSetContext(scope, target.name)
	if err != nil {
		return scope, err
	}
	scope.Component = component
	if scope.ComponentType == "" {
		scope.ComponentType = componentType
	}
	return scope, nil
}

func findGlobalSetContext(scope secretScope, name string) (string, string, error) {
	entries, _, err := enumerateScopesFn(secretScope{Stack: scope.Stack, ComponentType: scope.ComponentType})
	if err != nil {
		return "", "", componentRequiredForSet(name, fmt.Sprintf("the global declaration could not be verified: %v", err))
	}
	var selected *secrets.Declaration
	var component, componentType string
	for _, entry := range entries {
		if entry.Stack != "" && entry.Stack != scope.Stack {
			continue
		}
		if scope.ComponentType != "" && entry.ComponentType != "" && entry.ComponentType != scope.ComponentType {
			continue
		}
		decl, ok := secrets.ExtractDeclarations(entry.Section)[name]
		if !ok {
			continue
		}
		if decl.Scope != secrets.ScopeGlobal {
			return "", "", componentRequiredForSet(name, "the declaration is not global")
		}
		if selected != nil && decl != *selected {
			return "", "", componentRequiredForSet(name, "global declarations differ between components")
		}
		copy := decl
		selected = &copy
		if component == "" {
			component, componentType = entry.Component, entry.ComponentType
		}
	}
	if selected == nil {
		return "", "", componentRequiredForSet(name, "no global declaration was found in the stack")
	}
	return component, componentType, nil
}

func componentRequiredForSet(name, reason string) error {
	return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
		WithExplanationf("--component is required to set secret %q: %s", name, reason).
		WithHint("Omit --component only for a secret declared with `scope: global`; otherwise specify --component or -c").
		Err()
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
		return setTargetFromArg(args[0])
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

func setTargetFromArg(arg string) (setTarget, error) {
	name, value, hasValue := strings.Cut(arg, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return setTarget{}, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("secret NAME is required").Err()
	}
	return setTarget{name: name, value: value, hasValue: hasValue}, nil
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
