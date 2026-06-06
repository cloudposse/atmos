package secret

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

var initParser *flags.StandardParser

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Provision declared secrets, prompting for any that are missing.",
	Long:  "Scan the declared secrets for a component in a stack and interactively prompt for each required secret that is not yet initialized, writing the entered values to the configured backend.",
	Args:  cobra.NoArgs,
	RunE:  runSecretInit,
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

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	// First-time DX: offer to generate key material for any key-generating vault (e.g. SOPS) that
	// lacks it, so the value prompts below can actually encrypt/store.
	if err := offerKeygen(svc, dryRun); err != nil {
		return err
	}

	initialized, err := initDeclaredSecrets(svc, force, dryRun)
	if err != nil {
		return err
	}

	if dryRun {
		ui.Infof("Dry run: %d secret(s) would be initialized", initialized)
		return nil
	}
	ui.Successf("Initialized %d secret(s) for component `%s` in stack `%s`", initialized, scope.Component, scope.Stack)
	return nil
}

// initDeclaredSecrets walks the declared secrets and provisions each that is missing (or all, with
// force), prompting for values. In dry-run mode it only reports. Returns the count handled.
func initDeclaredSecrets(svc *secrets.Service, force, dryRun bool) (int, error) {
	statuses := svc.Status()
	initialized := 0
	for i := range statuses {
		st := &statuses[i]
		if !force && st.Initialized {
			continue
		}
		if dryRun {
			ui.Infof("Would initialize `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
			initialized++
			continue
		}

		ui.Infof("Initializing `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
		value, promptErr := promptForSecretValue()
		if promptErr != nil {
			return initialized, promptErr
		}
		if err := svc.Set(st.Declaration.Name, value); err != nil {
			return initialized, err
		}
		initialized++
	}
	return initialized, nil
}

// offerKeygen detects vaults whose backend can generate key material but has none yet, and offers
// to generate before the value-prompt loop. Backend-agnostic (any provider implementing the keygen
// capability). In dry-run mode it only reports what it would generate.
func offerKeygen(svc *secrets.Service, dryRun bool) error {
	missing, err := svc.VaultsMissingKeys()
	if err != nil {
		return err
	}
	for _, vault := range missing {
		if dryRun {
			ui.Infof("Would generate key material for vault `%s`", vault.Name)
			continue
		}
		confirmed, confirmErr := confirmAction("Vault `" + vault.Name + "` has no key material. Generate it now?")
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
