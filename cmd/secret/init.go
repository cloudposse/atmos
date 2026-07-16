package secret

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

var initParser *flags.StandardParser

var initStdinIsTTY = func() bool { return terminal.New().IsTTY(terminal.Stdin) }

type initOptions struct {
	force  bool
	dryRun bool
	values map[string]string
	mode   string
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Provision (and rotate) declared secrets, from prompts or dotenv input.",
	Long: "Walk the declared secrets for a stack and interactively initialize or rotate them. With " +
		"--stack alone, the whole stack is provisioned: stack-scoped secrets once each plus every " +
		"instance's instance-scoped secrets. With --component, only that instance is provisioned. " +
		"Already-initialized secrets prompt to update (rotate) or skip; --force rotates them all. " +
		"When stdin is redirected, values are read as dotenv KEY=VALUE entries.",
	Args: cobra.NoArgs,
	RunE: runSecretInit,
}

func init() {
	initParser = flags.NewStandardParser(
		flags.WithBoolFlag("all", "", false, "Initialize declared secrets across every stack"),
		flags.WithBoolFlag("force", "f", false, "Re-prompt and overwrite already-initialized secrets"),
		flags.WithBoolFlag("dry-run", "", false, "Show what would be initialized without prompting or writing"),
		flags.WithStringFlag("input", "", "", "Env file to initialize from (use - for stdin)"),
		flags.WithStringFlag("mode", "", "warn", "Input handling mode: warn or strict for undeclared keys"),
	)
	initParser.RegisterFlags(initCmd)
}

func runSecretInit(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretInit")()

	all, _ := cmd.Flags().GetBool("all")
	facet, err := parseInitScope(cmd, args, all)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	input, _ := cmd.Flags().GetString("input")
	mode, _ := cmd.Flags().GetString("mode")

	values, err := readInitInput(input)
	if err != nil {
		return err
	}

	opts := initOptions{force: force, dryRun: dryRun, values: values, mode: mode}
	if all {
		if facet.Component != "" {
			return errUtils.Build(errUtils.ErrValidationFailed).
				WithExplanation("--all cannot be combined with --component").
				WithHint("Use --all by itself, or specify --stack and --component for one instance").
				Err()
		}
		return initAllScopes(facet, opts)
	}

	// Single instance when --component is given; otherwise the whole stack.
	if facet.Component != "" {
		return initSingleScope(facet, opts)
	}
	return initWholeStack(facet, opts)
}

// readInitInput uses an explicit --input path when supplied. Otherwise, redirected stdin is a
// dotenv source, enabling both `atmos secret init < .env.local` and `cat .env.local | atmos secret init`.
// An empty redirected stream preserves the normal interactive behavior.
func readInitInput(input string) (map[string]string, error) {
	if input != "" {
		return parseSecretsFile(input, "env")
	}
	if initStdinIsTTY() {
		return nil, nil
	}
	values, err := parseEnvSecrets(os.Stdin)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

// parseInitScope reads the facets for init. --stack is required (prompted on a TTY); --component is
// optional — omitting it provisions every instance in the stack.
func parseInitScope(cmd *cobra.Command, args []string, all bool) (secretScope, error) {
	facet, err := parseFacets(cmd)
	if err != nil {
		return facet, err
	}
	if all {
		if facet.Stack != "" {
			return facet, errUtils.Build(errUtils.ErrValidationFailed).
				WithExplanation("--all cannot be combined with --stack").
				WithHint("Use --all for every stack, or omit --all to initialize one stack").
				Err()
		}
		return facet, nil
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
func initSingleScope(scope secretScope, opts initOptions) error {
	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}
	if err := validateInitInput([]secretService{svc}, opts.values, opts.mode); err != nil {
		return err
	}
	if err := offerKeygen(svc, opts.dryRun); err != nil {
		return err
	}
	n, err := rotateDeclaredSecrets(svc, scope.Stack, nil, opts)
	if err != nil {
		return err
	}
	reportInit(n, opts.dryRun)
	return nil
}

// initWholeStack provisions/rotates every instance in the stack: stack-scoped secrets once each
// plus each instance's instance-scoped secrets. Stack-scoped secrets are de-duplicated across
// instances so they are only prompted once.
func initWholeStack(facet secretScope, opts initOptions) error {
	entries, _, err := enumerateScopesFn(secretScope{Stack: facet.Stack, Identity: facet.Identity})
	if err != nil {
		return err
	}
	return initEntries(facet, entries, opts)
}

// initAllScopes provisions every declared secret instance across every stack.
func initAllScopes(facet secretScope, opts initOptions) error {
	entries, _, err := enumerateScopesFn(secretScope{Identity: facet.Identity})
	if err != nil {
		return err
	}
	return initEntries(facet, entries, opts)
}

func initEntries(facet secretScope, entries []scopeEntry, opts initOptions) error {
	if len(entries) == 0 {
		ui.Info(emptyListMessage(facet))
		return nil
	}

	type initTarget struct {
		stack string
		svc   secretService
	}
	targets := make([]initTarget, 0, len(entries))
	for _, entry := range entries {
		scope := secretScope{Stack: entry.Stack, Component: entry.Component, Identity: facet.Identity}
		svc, loadErr := loadServiceFn(scope)
		if loadErr != nil {
			return loadErr
		}
		targets = append(targets, initTarget{stack: entry.Stack, svc: svc})
	}
	services := make([]secretService, 0, len(targets))
	for _, target := range targets {
		services = append(services, target.svc)
	}
	if err := validateInitInput(services, opts.values, opts.mode); err != nil {
		return err
	}

	seenStackScoped := make(map[string]bool)
	total := 0
	for _, target := range targets {
		svc := target.svc
		if err := offerKeygen(svc, opts.dryRun); err != nil {
			return err
		}
		n, rotateErr := rotateDeclaredSecrets(svc, target.stack, seenStackScoped, opts)
		if rotateErr != nil {
			return rotateErr
		}
		total += n
	}
	reportInit(total, opts.dryRun)
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
func rotateDeclaredSecrets(svc secretService, stackName string, seen map[string]bool, opts initOptions) (int, error) {
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

		handled, err := provisionSecret(svc, st, opts)
		if err != nil {
			return count, err
		}
		if handled {
			count++
		}
	}
	return count, nil
}

// validateInitInput resolves input keys across every selected service before writes start. Default
// warn mode mirrors dotenv's permissive workflow; strict mode turns an undeclared key into an error.
func validateInitInput(services []secretService, values map[string]string, mode string) error {
	if values == nil {
		return nil
	}
	if mode != "warn" && mode != "strict" {
		return errUtils.Build(errUtils.ErrValidationFailed).
			WithExplanationf("unsupported init input mode %q", mode).
			WithHint("Use --mode=warn or --mode=strict").
			Err()
	}
	for _, name := range sortedKeys(values) {
		declared := false
		for _, svc := range services {
			if svc.IsDeclared(name) {
				declared = true
				break
			}
		}
		if declared {
			continue
		}
		if mode == "strict" {
			return errUtils.Build(errUtils.ErrValidationFailed).
				WithExplanationf("input key %q is not declared as a secret", name).
				WithHint("Declare it under the selected component's secrets.vars or remove it from the input").
				Err()
		}
		ui.Warningf("Skipping undeclared input key `%s`", name)
	}
	return nil
}

// provisionSecret provisions a single declared secret: in dry-run it only reports; an already-set
// secret prompts to update (rotate) or skip unless force; otherwise it prompts for a value and
// stores it. Returns whether the secret was handled (and should be counted).
//
//nolint:revive // This keeps the prompt, dry-run, and dotenv initialization paths together.
func provisionSecret(svc secretService, st *secrets.Status, opts initOptions) (bool, error) {
	value, hasValue := opts.values[st.Declaration.Name]
	if opts.values != nil && !hasValue {
		return false, nil
	}
	if opts.dryRun {
		ui.Infof("Would %s `%s` (%s)", initVerb(st.Initialized), st.Declaration.Name, backendLabel(&st.Declaration))
		return true, nil
	}

	// Already set → offer update/skip unless force rotates everything.
	if st.Initialized && !opts.force {
		if opts.values != nil {
			return false, nil
		}
		confirmed, confirmErr := confirmActionFn(fmt.Sprintf("Secret `%s` is already set. Update (rotate) it?", st.Declaration.Name))
		if confirmErr != nil {
			return false, confirmErr
		}
		if !confirmed {
			return false, nil
		}
	}

	ui.Infof("Setting `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
	if opts.values == nil {
		var promptErr error
		value, promptErr = promptForValueFn()
		if promptErr != nil {
			return false, promptErr
		}
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
