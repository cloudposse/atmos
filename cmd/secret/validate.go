package secret

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate that all required declared secrets are initialized.",
	Long:  "Validate that every required declared secret is initialized in its backend. Exits non-zero (for CI) when required secrets are missing.",
	Args:  cobra.NoArgs,
	RunE:  runSecretValidate,
}

func init() {
	internal.RegisterCommandMetadata(
		validateCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/secret/validate"),
	)
}

func runSecretValidate(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretValidate")()

	scope, err := parseScopeStack(cmd, args)
	if err != nil {
		return err
	}

	// Guard the advanced SOPS `spec.file` path: a hand-written template that doesn't discriminate by
	// component would silently collide instance secrets or break stack-secret sharing.
	if err := checkStackSopsCollisions(scope); err != nil {
		return err
	}

	result, err := validateScope(scope)
	if err != nil {
		return err
	}
	if result.Valid() {
		if scope.Component == "" {
			ui.Successf("All required stack-scoped and global secrets are initialized in stack `%s`", scope.Stack)
		} else {
			ui.Successf("All required secrets are initialized for component `%s` in stack `%s`", scope.Component, scope.Stack)
		}
		return nil
	}

	for i := range result.MissingRequired {
		st := &result.MissingRequired[i]
		ui.Errorf("Missing required secret `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
	}
	for i := range result.Errored {
		st := &result.Errored[i]
		ui.Warningf("Could not determine status of secret `%s`: %v", st.Declaration.Name, st.Err)
	}

	return errUtils.Build(errUtils.ErrValidationFailed).
		WithExplanation(fmt.Sprintf("%d required secret(s) missing", len(result.MissingRequired))).
		Err()
}

// validateScope checks only shared declarations when no component was requested. Instance
// declarations still require an explicit component and are never sent to a backend on this path.
func validateScope(scope secretScope) (secrets.ValidationResult, error) {
	if scope.Component != "" {
		svc, err := loadServiceFn(scope)
		if err != nil {
			return secrets.ValidationResult{}, err
		}
		return svc.Validate(), nil
	}
	entries, _, err := enumerateScopesFn(scope)
	if err != nil {
		return secrets.ValidationResult{}, err
	}
	var result secrets.ValidationResult
	for _, entry := range entries {
		if scope.ComponentType != "" && entry.ComponentType != scope.ComponentType {
			continue
		}
		if len(secrets.ExtractDeclarations(sharedSecretSection(entry.Section))) == 0 {
			continue
		}
		svc, err := loadServiceFn(secretScope{Stack: entry.Stack, Component: entry.Component, ComponentType: entry.ComponentType, Identity: scope.Identity, SharedOnly: true})
		if err != nil {
			return result, err
		}
		// Validate each inherited context: component provider/auth overrides can differ.
		validated := svc.Validate()
		result.MissingRequired = append(result.MissingRequired, validated.MissingRequired...)
		result.Errored = append(result.Errored, validated.Errored...)
	}
	return result, nil
}

// sharedSecretSection copies declaration maps before filtering, preserving the caller's section.
func sharedSecretSection(section map[string]any) map[string]any {
	result := make(map[string]any, len(section))
	for key, value := range section {
		result[key] = value
	}
	secretSection, _ := section["secrets"].(map[string]any)
	shared := make(map[string]any, len(secretSection))
	for key, value := range secretSection {
		shared[key] = value
	}
	declarations, _ := secretSection["vars"].(map[string]any)
	vars := map[string]any{}
	for name, declaration := range secrets.ExtractDeclarations(section) {
		if declaration.Scope == secrets.ScopeStack || declaration.Scope == secrets.ScopeGlobal {
			vars[name] = declarations[name]
		}
	}
	shared["vars"] = vars
	result["secrets"] = shared
	return result
}
