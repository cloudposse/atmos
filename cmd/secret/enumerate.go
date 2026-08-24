package secret

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// scopeEntry is a single (stack, component) instance that declares one or more secrets, paired
// with its resolved component section (declarations carry their derived scope after stack merge).
type scopeEntry struct {
	Stack     string
	Component string
	Section   map[string]any
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
	stacksMap, err := e.ExecuteDescribeStacksWithAuthDisabled(
		&atmosConfig, facet.Stack, components, nil, nil,
		false, true, true, false, credentialFreeSkip(), nil, true,
	)
	if err != nil {
		return nil, nil, err
	}

	return collectSecretScopeEntries(stacksMap, facet.Component), &atmosConfig, nil
}

// collectSecretScopeEntries traverses the describe-stacks map
// (stack -> components -> <type> -> component -> section) and keeps the instances that declare
// secrets, optionally narrowed to a single component. Entries are sorted by stack then component.
func collectSecretScopeEntries(stacksMap map[string]any, componentFilter string) []scopeEntry {
	var entries []scopeEntry
	for stackName, raw := range stacksMap {
		stackMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, secretEntriesInStack(stackName, stackMap, componentFilter)...)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Stack != entries[j].Stack {
			return entries[i].Stack < entries[j].Stack
		}
		return entries[i].Component < entries[j].Component
	})
	return entries
}

// secretEntriesInStack returns the secret-declaring instances within a single stack's describe map
// (components -> <type> -> component -> section), optionally narrowed to componentFilter.
func secretEntriesInStack(stackName string, stackMap map[string]any, componentFilter string) []scopeEntry {
	comps, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return nil
	}
	var entries []scopeEntry
	for _, typeRaw := range comps {
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
			entries = append(entries, scopeEntry{Stack: stackName, Component: compName, Section: section})
		}
	}
	return entries
}

// stackCompletion returns the distinct stacks that declare secrets. It backs both shell completion
// and the interactive prompt for a missing --stack.
func stackCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	entries, _, err := enumerateScopesFn(secretScope{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return distinct(entries, func(e scopeEntry) string { return e.Stack }), cobra.ShellCompDirectiveNoFileComp
}

// componentCompletion returns the distinct components that declare secrets in the currently
// selected --stack (read from viper, so it reflects a value just chosen via the stack prompt).
func componentCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	entries, _, err := enumerateScopesFn(secretScope{Stack: viper.GetString("stack")})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return distinct(entries, func(e scopeEntry) string { return e.Component }), cobra.ShellCompDirectiveNoFileComp
}

// checkStackSopsCollisions enumerates a stack's secret-declaring instances and verifies their SOPS
// files don't collide across scopes (distinct instances sharing a file, or a stack-scoped secret
// resolving per-component). It is a write-time guardrail for the advanced `spec.file` template path.
func checkStackSopsCollisions(stack string) error {
	defer perf.Track(nil, "secret.checkStackSopsCollisions")()

	entries, atmosConfig, err := enumerateScopesFn(secretScope{Stack: stack})
	if err != nil {
		return err
	}
	var placements []secrets.SopsPlacement
	for _, entry := range entries {
		svc := secrets.NewService(atmosConfig, entry.Stack, entry.Component, entry.Section)
		placements = append(placements, svc.SopsPlacements()...)
	}
	return secrets.DetectSopsCollisions(placements)
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
