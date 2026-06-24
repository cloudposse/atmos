package secret

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

var initParser *flags.StandardParser

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Provision (and rotate) declared secrets, prompting for each.",
	Long: "Walk the declared secrets for a stack and interactively initialize or rotate them. With " +
		"--stack alone, the whole stack is provisioned: stack-scoped secrets once each plus every " +
		"instance's instance-scoped secrets. With --component, only that instance is provisioned. " +
		"Already-initialized secrets prompt to update (rotate) or skip; --force rotates them all.",
	Args: cobra.NoArgs,
	RunE: runSecretInit,
}

func init() {
	initParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Re-prompt and overwrite already-initialized secrets"),
		flags.WithBoolFlag("dry-run", "", false, "Show what would be initialized without prompting or writing"),
	)
	initParser.RegisterFlags(initCmd)
}

func runSecretInit(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretInit")()

	facet, err := parseInitScope(cmd, args)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Single instance when --component is given; otherwise the whole stack.
	if facet.Component != "" {
		return initSingleScope(facet, force, dryRun)
	}
	return initWholeStack(facet, force, dryRun)
}

// parseInitScope reads the facets for init. --stack is required (prompted on a TTY); --component is
// optional — omitting it provisions every instance in the stack.
func parseInitScope(cmd *cobra.Command, args []string) (secretScope, error) {
	facet, err := parseFacets(cmd)
	if err != nil {
		return facet, err
	}
	if facet.Stack == "" {
		chosen, promptErr := flags.PromptForMissingRequired("stack", "Choose a stack", stackCompletion, cmd, args)
		if promptErr != nil {
			return facet, promptErr
		}
		facet.Stack = chosen
	}
	if facet.Stack == "" {
		return facet, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack is required for secret operations").
			WithHint("Specify a stack with --stack or -s").Err()
	}
	viper.GetViper().Set("stack", facet.Stack)
	return facet, nil
}

// initSingleScope provisions/rotates the declared secrets for one (stack, component) instance.
func initSingleScope(scope secretScope, force, dryRun bool) error {
	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}
	if err := offerKeygen(svc, dryRun); err != nil {
		return err
	}
	n, err := rotateDeclaredSecrets(svc, scope.Stack, force, dryRun, nil)
	if err != nil {
		return err
	}
	reportInit(n, dryRun)
	return nil
}

// initWholeStack provisions/rotates every instance in the stack: stack-scoped secrets once each
// plus each instance's instance-scoped secrets. Stack-scoped secrets are de-duplicated across
// instances so they are only prompted once.
func initWholeStack(facet secretScope, force, dryRun bool) error {
	entries, _, err := enumerateScopesFn(secretScope{Stack: facet.Stack, Identity: facet.Identity})
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		ui.Info(emptyListMessage(facet))
		return nil
	}

	seenStackScoped := make(map[string]bool)
	total := 0
	for _, entry := range entries {
		scope := secretScope{Stack: entry.Stack, Component: entry.Component, Identity: facet.Identity}
		svc, loadErr := loadServiceFn(scope)
		if loadErr != nil {
			return loadErr
		}
		if err := offerKeygen(svc, dryRun); err != nil {
			return err
		}
		n, rotateErr := rotateDeclaredSecrets(svc, entry.Stack, force, dryRun, seenStackScoped)
		if rotateErr != nil {
			return rotateErr
		}
		total += n
	}
	reportInit(total, dryRun)
	return nil
}

// reportInit prints the final summary for an init run.
func reportInit(count int, dryRun bool) {
	if dryRun {
		ui.Infof("Dry run: %d secret(s) would be initialized or rotated", count)
		return
	}
	ui.Successf("Initialized or rotated %d secret(s)", count)
}

// rotateDeclaredSecrets walks the declared secrets for a service and provisions each: missing
// secrets are prompted and set; already-initialized secrets prompt to update (rotate) or skip
// unless --force rotates them all. In dry-run mode it only reports. When seen is non-nil (whole-
// stack mode), stack-scoped secrets are processed once per (stack, name). Returns the count handled.
func rotateDeclaredSecrets(svc secretService, stackName string, force, dryRun bool, seen map[string]bool) (int, error) {
	// init provisions secrets, so it needs an authoritative initialized/not-initialized answer
	// from every backend (verify=true). Local backends (e.g. SOPS) stay credential-free.
	statuses := svc.Status(true)
	count := 0
	for i := range statuses {
		st := &statuses[i]

		// In whole-stack mode, a stack-scoped secret is inherited by every instance — handle it once.
		if seen != nil && st.Declaration.IsStackScoped() {
			key := stackName + "\x00" + st.Declaration.Name
			if seen[key] {
				continue
			}
			seen[key] = true
		}

		handled, err := provisionSecret(svc, st, force, dryRun)
		if err != nil {
			return count, err
		}
		if handled {
			count++
		}
	}
	return count, nil
}

// provisionSecret provisions a single declared secret: in dry-run it only reports; an already-set
// secret prompts to update (rotate) or skip unless force; otherwise it prompts for a value and
// stores it. Returns whether the secret was handled (and should be counted).
func provisionSecret(svc secretService, st *secrets.Status, force, dryRun bool) (bool, error) {
	if dryRun {
		ui.Infof("Would %s `%s` (%s)", initVerb(st.Initialized), st.Declaration.Name, backendLabel(&st.Declaration))
		return true, nil
	}

	// Already set → offer update/skip unless force rotates everything.
	if st.Initialized && !force {
		confirmed, confirmErr := confirmActionFn(fmt.Sprintf("Secret `%s` is already set. Update (rotate) it?", st.Declaration.Name))
		if confirmErr != nil {
			return false, confirmErr
		}
		if !confirmed {
			return false, nil
		}
	}

	ui.Infof("Setting `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
	value, promptErr := promptForValueFn()
	if promptErr != nil {
		return false, promptErr
	}
	if err := svc.Set(st.Declaration.Name, value); err != nil {
		return false, err
	}
	return true, nil
}

// initVerb returns the dry-run verb for a secret based on whether it is already initialized.
func initVerb(initialized bool) string {
	if initialized {
		return "rotate"
	}
	return "initialize"
}

// offerKeygen detects vaults whose backend can generate key material but has none yet, and offers
// to generate before the value-prompt loop. Backend-agnostic (any provider implementing the keygen
// capability). In dry-run mode it only reports what it would generate.
func offerKeygen(svc secretService, dryRun bool) error {
	missing, err := svc.VaultsMissingKeys()
	if err != nil {
		return err
	}
	for _, vault := range missing {
		if dryRun {
			ui.Infof("Would generate key material for vault `%s`", vault.Name)
			continue
		}
		confirmed, confirmErr := confirmActionFn("Vault `" + vault.Name + "` has no key material. Generate it now?")
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			ui.Warningf("Skipping key generation for `%s`; setting its secrets may fail until it has a key.", vault.Name)
			continue
		}
		res, genErr := svc.GenerateKeyForVault(vault)
		if genErr != nil {
			return genErr
		}
		ui.Successf("Vault `%s` (%s): %s", res.Vault, res.Kind, res.Summary)
		for _, out := range res.Outputs {
			ui.Infof("%s → %s", out.Label, out.Location)
		}
	}
	return nil
}
