package secret

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// scopeEntry is a single (stack, component) instance that declares one or more secrets, paired
// with its resolved component section (declarations carry their derived scope after stack merge).
type scopeEntry struct {
	Stack         string
	Component     string
	ComponentType string
	Section       map[string]any
}

// enumerateScopesFn is a seam so tests can inject scope entries without real stack processing.
var enumerateScopesFn = enumerateSecretScopes

// enumerateSecretScopes lists every (stack, component) instance that declares secrets, narrowed by
// the given facets (an empty Stack/Component means "all"). It resolves the stack manifests once via
// describe-stacks with auth disabled and all credentialed read functions skipped (see
// credentialFreeSkip) — declarations and their derived scope are available without retrieving any
// secret values or reading any remote backend.
func enumerateSecretScopes(facet secretScope) ([]scopeEntry, *schema.AtmosConfiguration, error) {
	defer perf.Track(nil, "secret.enumerateSecretScopes")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: facet.Stack}, true)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	var components []string
	if facet.Component != "" {
		components = []string{facet.Component}
	}

	// Listing only reads the static `secrets.vars` declarations (see collectSecretScopeEntries →
	// secrets.ExtractDeclarations); it never retrieves a secret value, so per-component auth is
	// pure overhead. Disable it explicitly so a 72-component stack doesn't run 72 auth cycles
	// (credentials-file rewrite + keyring rebuild) just to enumerate declarations.
	//
	// With auth disabled, every credentialed read function must also be skipped: an evaluated
	// `!terraform.state`/`!terraform.output`/`!store` would fall back to the default AWS chain and
	// fail (e.g. an unreachable EC2 IMDS endpoint) even though enumeration never needs the resolved
	// value. See credentialFreeSkip.
	stacksMap, err := describeSecretStacks(&atmosConfig, facet.Stack, components)
	if err != nil {
		return enumerateFailure(&atmosConfig, facet, err)
	}

	return collectSecretScopeEntries(stacksMap, facet.Component), &atmosConfig, nil
}

// describeSecretStacks resolves declarations for the given stack and components credential-free.
// It limits template evaluation as well as YAML tags: unrelated atmos.Component expressions can
// fetch live outputs even when credentialed tags are skipped.
func describeSecretStacks(atmosConfig *schema.AtmosConfiguration, stack string, components []string) (map[string]any, error) {
	return e.ExecuteDescribeStacksWithOptions(
		atmosConfig, stack, components, nil, nil,
		false, true, true, false, credentialFreeSkip(), nil, true,
		secretDeclarationEvaluation(),
	)
}

// enumerationError reports the instances that could not be evaluated while enumerating. The
// entries that did evaluate are still returned alongside it, so callers that can degrade (listing,
// completion) use the partial result while callers that need every instance (the SOPS collision
// guard) treat it as the failure it is.
type enumerationError struct {
	failures []error
	// sopsFailures is the subset of failures whose instance declares a SOPS-backed secret. Only
	// those can hide a SOPS file collision, so only they make the collision guard fail closed.
	sopsFailures []error
}

func (e *enumerationError) Error() string { return errors.Join(e.failures...).Error() }

// Unwrap exposes every attributed failure to errors.Is / errors.As.
func (e *enumerationError) Unwrap() []error { return e.failures }

// isPartialEnumeration reports whether err only describes instances that failed to evaluate, in
// which case the entries returned with it are still valid.
func isPartialEnumeration(err error) bool {
	var partial *enumerationError
	return errors.As(err, &partial)
}

// declaresSops reports whether a (raw, unevaluated) section declares any SOPS-backed secret.
func declaresSops(section map[string]any) bool {
	for _, decl := range secrets.ExtractDeclarations(section) {
		if decl.BackendType == secrets.BackendSops {
			return true
		}
	}
	return false
}

// attributeScopeFailure names the component and stack whose evaluation failed.
func attributeScopeFailure(stack, component string, cause error) error {
	return errUtils.Build(ErrSecretScopeEvaluation).
		WithCausef("component %q in stack %q: %w", component, stack, cause).
		WithHintf("Reproduce with `atmos describe component %s --stack %s`. A dynamic template in a declaration field (for example `description: '{{ printf \"%%v\" .vars }}'`) makes Atmos evaluate that whole component, including unrelated atmos.Component calls.", component, stack).
		Err()
}

// enumerateFailure handles a failed stack-wide describe. When a component was requested the
// failure is attributed to it directly. Otherwise each declaring instance is evaluated on its own so
// one broken component is named and does not hide its healthy siblings: the healthy entries are
// returned together with an *enumerationError describing the broken ones.
func enumerateFailure(atmosConfig *schema.AtmosConfiguration, facet secretScope, cause error) ([]scopeEntry, *schema.AtmosConfiguration, error) {
	if facet.Component != "" {
		return nil, nil, attributeScopeFailure(facet.Stack, facet.Component, cause)
	}

	// Discover which instances declare secrets without evaluating any template or function.
	raw, err := e.ExecuteDescribeStacksWithOptions(atmosConfig, facet.Stack, nil, nil, nil,
		false, false, false, false, nil, nil, true, e.DescribeStacksErrorOptions{})
	if err != nil {
		return nil, nil, cause
	}

	var entries []scopeEntry
	var failures, sopsFailures []error
	for _, candidate := range collectSecretScopeEntries(raw, "") {
		stacksMap, describeErr := describeSecretStacks(atmosConfig, candidate.Stack, []string{candidate.Component})
		if describeErr != nil {
			failure := attributeScopeFailure(candidate.Stack, candidate.Component, describeErr)
			failures = append(failures, failure)
			if declaresSops(candidate.Section) {
				sopsFailures = append(sopsFailures, failure)
			}
			continue
		}
		entries = append(entries, collectSecretScopeEntries(stacksMap, candidate.Component)...)
	}
	sortScopeEntries(entries)
	if len(failures) == 0 {
		// The failure came from an instance that declares no secrets; it is irrelevant here.
		return entries, atmosConfig, nil
	}
	return entries, atmosConfig, &enumerationError{failures: failures, sopsFailures: sopsFailures}
}

// collectSecretScopeEntries traverses the describe-stacks map
// (stack -> components -> <type> -> component -> section) and keeps the instances that declare
// secrets, optionally narrowed to a single component. Entries are sorted by stack, component,
// then component type so a name shared across component types has deterministic ordering.
func collectSecretScopeEntries(stacksMap map[string]any, componentFilter string) []scopeEntry {
	var entries []scopeEntry
	for stackName, raw := range stacksMap {
		stackMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, secretEntriesInStack(stackName, stackMap, componentFilter)...)
	}

	sortScopeEntries(entries)
	return entries
}

// sortScopeEntries orders entries by stack, component, then component type.
func sortScopeEntries(entries []scopeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Stack != entries[j].Stack {
			return entries[i].Stack < entries[j].Stack
		}
		if entries[i].Component != entries[j].Component {
			return entries[i].Component < entries[j].Component
		}
		return entries[i].ComponentType < entries[j].ComponentType
	})
}

// secretEntriesInStack returns the secret-declaring instances within a single stack's describe map
// (components -> <type> -> component -> section), optionally narrowed to componentFilter.
func secretEntriesInStack(stackName string, stackMap map[string]any, componentFilter string) []scopeEntry {
	comps, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return nil
	}
	var entries []scopeEntry
	for componentType, typeRaw := range comps {
		typeMap, ok := typeRaw.(map[string]any)
		if !ok {
			continue
		}
		for compName, secRaw := range typeMap {
			if componentFilter != "" && compName != componentFilter {
				continue
			}
			section, ok := secRaw.(map[string]any)
			if !ok {
				continue
			}
			if len(secrets.ExtractDeclarations(section)) == 0 {
				continue
			}
			entries = append(entries, scopeEntry{Stack: stackName, Component: compName, ComponentType: componentType, Section: section})
		}
	}
	return entries
}

// stackCompletion returns the distinct stacks that declare secrets. It backs both shell completion
// and the interactive prompt for a missing --stack.
func stackCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	entries, _, err := enumerateScopesFn(secretScope{})
	if err != nil && !isPartialEnumeration(err) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return distinct(entries, func(e scopeEntry) string { return e.Stack }), cobra.ShellCompDirectiveNoFileComp
}

// componentCompletionForStack returns a completion function scoped to the given stack, filtering
// directly on the caller's resolved value instead of reading a stack from viper. Used by the
// missing --component prompt (requireScopeComponent), which already knows the resolved stack
// (whether it came from a flag or was just chosen interactively) — passing it in directly avoids
// needing to mirror it into global viper state, which would leak across command invocations (see
// docs/fixes for the incident this replaced).
func componentCompletionForStack(stack string) flags.CompletionFunc {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		entries, _, err := enumerateScopesFn(secretScope{Stack: stack})
		if err != nil && !isPartialEnumeration(err) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return distinct(entries, func(e scopeEntry) string { return e.Component }), cobra.ShellCompDirectiveNoFileComp
	}
}

// checkStackSopsCollisions enumerates a stack's secret-declaring instances and verifies their SOPS
// files don't collide across scopes (distinct instances sharing a file, or a stack-scoped secret
// resolving per-component). It is a write-time guardrail for the advanced `spec.file` template path.
//
// Instances are enumerated credential-free, so a SOPS declaration whose `sops:` is a backend
// selector (for example `!aws.cloudformation.output ...`) is resolved here lazily, with the
// caller's identity, only for instances that have one. A selector that cannot be resolved fails
// the check closed — naming the declaration, component and stack — rather than being skipped,
// because an unknown file could still collide with another instance's.
func checkStackSopsCollisions(scope secretScope) error {
	defer perf.Track(nil, "secret.checkStackSopsCollisions")()

	entries, atmosConfig, err := enumerateScopesFn(secretScope{Stack: scope.Stack})
	if err != nil {
		// An instance that could not be evaluated and declares no SOPS secret cannot collide, so
		// it does not block the check; one that does declare SOPS is an unknown file: fail closed.
		var partial *enumerationError
		if !errors.As(err, &partial) || len(partial.sopsFailures) > 0 {
			return err
		}
	}
	var placements []secrets.SopsPlacement
	var unresolved []error
	for _, entry := range entries {
		svc := secrets.NewService(atmosConfig, entry.Stack, entry.Component, entry.Section,
			secrets.WithSelectorEvaluator(entrySelectorEvaluatorFn(secretScope{
				Stack:         entry.Stack,
				Component:     entry.Component,
				ComponentType: entry.ComponentType,
				Identity:      scope.Identity,
			})))
		entryPlacements, placementErr := svc.SopsPlacements()
		placements = append(placements, entryPlacements...)
		if placementErr != nil {
			unresolved = append(unresolved, placementErr)
		}
	}
	return errors.Join(secrets.DetectSopsCollisions(placements), errors.Join(unresolved...))
}

// distinct returns the sorted, de-duplicated values produced by key over the entries.
func distinct(entries []scopeEntry, key func(scopeEntry) string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, entry := range entries {
		v := key(entry)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
