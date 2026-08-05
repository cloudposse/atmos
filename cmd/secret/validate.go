package secret

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate that all required declared secrets are initialized.",
	Long:  "Validate that every required declared secret is initialized in its backend. Exits non-zero (for CI) when required secrets are missing.",
	Args:  cobra.NoArgs,
	RunE:  runSecretValidate,
}

func runSecretValidate(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretValidate")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}

	// Guard the advanced SOPS `spec.file` path: a hand-written template that doesn't discriminate by
	// component would silently collide instance secrets or break stack-secret sharing.
	if err := checkStackSopsCollisions(scope.Stack); err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	result := svc.Validate()
	if result.Valid() {
		ui.Successf("All required secrets are initialized for component `%s` in stack `%s`", scope.Component, scope.Stack)
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
