package secret

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// Stack-scoped and global secrets live at a coordinate that does not depend on a component, so the
// commands that operate on them accept just --stack. A component is still loaded internally (every
// declaration is rendered in some component's context), but only as that loading context: it is
// never part of the backend coordinate and is never shown to the user as the target. Instance-scoped
// secrets keep requiring --component.

// parseScopeForName resolves the scope for a command that targets one named secret. With --component
// it behaves like parseScope. Without it, the name must resolve to one consistent stack/global
// declaration in the stack; otherwise the component is requested (an interactive prompt on a TTY, a
// "required flag" error everywhere else).
func parseScopeForName(cmd *cobra.Command, args []string, name, op string) (secretScope, error) {
	scope, err := parseScopeStack(cmd, args)
	if err != nil {
		return scope, err
	}
	if scope.Component != "" {
		return scope, nil
	}
	if name == "" {
		return requireScopeComponent(scope, cmd, args)
	}
	component, componentType, err := findSharedContext(scope, name, op)
	if err != nil {
		return promptForComponent(scope, cmd, args, err)
	}
	return adoptSharedContext(scope, component, componentType), nil
}

// parseScopeForStack resolves the scope for a command that operates on every declared secret of a
// scope. With --component it behaves like parseScope. Without it, the command operates on the
// stack's shared (stack and global) declarations only, which must agree across components;
// otherwise the component is requested as in parseScopeForName.
func parseScopeForStack(cmd *cobra.Command, args []string, op string) (secretScope, error) {
	scope, err := parseScopeStack(cmd, args)
	if err != nil {
		return scope, err
	}
	if scope.Component != "" {
		return scope, nil
	}
	component, componentType, err := findSharedStackContext(scope, op)
	if err != nil {
		return promptForComponent(scope, cmd, args, err)
	}
	return adoptSharedContext(scope, component, componentType), nil
}

// adoptSharedContext records the loading component on a scope and restricts the loaded service to
// the shared declarations, so an instance secret can never be read or written without --component.
func adoptSharedContext(scope secretScope, component, componentType string) secretScope {
	scope.Component = component
	if scope.ComponentType == "" {
		scope.ComponentType = componentType
	}
	scope.SharedOnly = true
	return scope
}

// promptForComponent is the fallback when no shared context could be resolved: choose a component
// interactively, or return the specific reason the component is required when there is nobody to ask.
func promptForComponent(scope secretScope, cmd *cobra.Command, args []string, cause error) (secretScope, error) {
	chosen, err := flags.PromptForMissingRequired("component", "Choose a component", componentCompletionForStack(scope.Stack), cmd, args)
	if err != nil {
		return scope, err
	}
	if chosen == "" {
		return scope, cause
	}
	scope.Component = chosen
	return scope, nil
}

// describeScope names what a command acted on: the stack for componentless (shared) scopes, or the
// component and stack otherwise.
func describeScope(scope secretScope) string {
	if scope.SharedOnly {
		return fmt.Sprintf("stack `%s` (stack-scoped and global secrets)", scope.Stack)
	}
	return fmt.Sprintf("component `%s` in stack `%s`", scope.Component, scope.Stack)
}

// findSharedContext locates the one consistent stack or global declaration of name in the stack and
// returns a component and component type that can load it. A component is still used internally to
// load the inherited declaration without changing the shared backend coordinate.
func findSharedContext(scope secretScope, name, op string) (string, string, error) {
	entries, _, err := enumerateScopesFn(secretScope{Stack: scope.Stack, ComponentType: scope.ComponentType})
	if err != nil {
		return "", "", errors.Join(
			componentRequiredForName(op, name, fmt.Sprintf("the shared declaration could not be verified: %v", err)),
			err,
		)
	}
	var selected *sharedDeclaration
	var component, componentType string
	for _, entry := range entries {
		if !sharedEntryMatches(scope, entry) {
			continue
		}
		decl, ok := secrets.ExtractDeclarations(entry.Section)[name]
		if !ok {
			continue
		}
		// Enumeration renders declarations in each component context using the configured Atmos
		// template delimiters. Compare those resolved declarations directly so component-less access
		// never selects one component's backend address arbitrarily and the guard remains independent
		// of template syntax.
		current := &sharedDeclaration{declaration: decl, provider: secrets.ExtractProviders(entry.Section)[decl.BackendName]}
		if err := validateSharedDeclaration(op, name, current, selected); err != nil {
			return "", "", err
		}
		selected = current
		if component == "" {
			component, componentType = entry.Component, entry.ComponentType
		}
	}
	if selected == nil {
		return "", "", componentRequiredForName(op, name, "no shared declaration was found in the stack")
	}
	return component, componentType, nil
}

// sharedDeclaration is a shared declaration together with the provider configuration it resolved to.
type sharedDeclaration struct {
	declaration secrets.Declaration
	provider    any
}

// findSharedStackContext locates a component context that renders every shared (stack and global)
// declaration of the stack, for commands that operate on all of a scope's secrets. Every shared
// declaration and its provider must agree across the components that declare it.
func findSharedStackContext(scope secretScope, op string) (string, string, error) {
	entries, _, err := enumerateScopesFn(secretScope{Stack: scope.Stack, ComponentType: scope.ComponentType})
	if err != nil {
		return "", "", errors.Join(
			componentRequiredForStack(op, fmt.Sprintf("the shared declarations could not be verified: %v", err)),
			err,
		)
	}
	union := map[string]sharedDeclaration{}
	perEntry := make([]map[string]sharedDeclaration, len(entries))
	for i, entry := range entries {
		if !sharedEntryMatches(scope, entry) {
			continue
		}
		providers := secrets.ExtractProviders(entry.Section)
		shared := map[string]sharedDeclaration{}
		for name, decl := range secrets.ExtractDeclarations(sharedSecretSection(entry.Section)) {
			current := sharedDeclaration{declaration: decl, provider: providers[decl.BackendName]}
			if previous, ok := union[name]; ok && !sameSharedDeclaration(&previous, &current) {
				return "", "", componentRequiredForStack(op, fmt.Sprintf("shared declaration %q or its provider differs between components", name))
			}
			union[name] = current
			shared[name] = current
		}
		perEntry[i] = shared
	}
	if len(union) == 0 {
		return "", "", componentRequiredForStack(op, "no stack-scoped or global secrets were found in the stack")
	}
	// Stack-level declarations render in every component, but a global declaration imported by only
	// some components does not. Use a component that renders all of them, or ask for one.
	for i, shared := range perEntry {
		if len(shared) == len(union) {
			return entries[i].Component, entries[i].ComponentType, nil
		}
	}
	return "", "", componentRequiredForStack(op, "no single component declares every shared secret in the stack")
}

// sameSharedDeclaration reports whether two shared declarations and their providers are identical.
func sameSharedDeclaration(a, b *sharedDeclaration) bool {
	return a.declaration == b.declaration && reflect.DeepEqual(a.provider, b.provider)
}

// sharedEntryMatches reports whether an enumerated entry belongs to the scope's stack and component type.
func sharedEntryMatches(scope secretScope, entry scopeEntry) bool {
	if entry.Stack != "" && entry.Stack != scope.Stack {
		return false
	}
	return scope.ComponentType == "" || entry.ComponentType == "" || entry.ComponentType == scope.ComponentType
}

// validateSharedDeclaration rejects an instance-scoped declaration, or one that differs from the
// previously selected shared declaration, because neither can be addressed without a component.
func validateSharedDeclaration(op, name string, current, selected *sharedDeclaration) error {
	if scope := current.declaration.Scope; scope != secrets.ScopeGlobal && scope != secrets.ScopeStack {
		return componentRequiredForName(op, name, "the declaration is instance-scoped")
	}
	if selected != nil && !sameSharedDeclaration(current, selected) {
		return componentRequiredForName(op, name, "shared declarations or providers differ between components")
	}
	return nil
}

// componentRequiredForName builds the "--component is required" error for a named secret.
func componentRequiredForName(op, name, reason string) error {
	return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
		WithExplanationf("--component is required to %s secret %q: %s", op, name, reason).
		WithHint("Omit --component for a consistent stack-scoped or global secret; instance secrets require --component or -c").
		Err()
}

// componentRequiredForStack builds the "--component is required" error for a whole-scope command.
func componentRequiredForStack(op, reason string) error {
	return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
		WithExplanationf("--component is required to %s: %s", op, reason).
		WithHint("Omit --component to operate on the stack's consistent stack-scoped and global secrets; instance secrets require --component or -c").
		Err()
}
