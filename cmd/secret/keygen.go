package secret

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/ui"
)

var keygenParser *flags.StandardParser

var keygenCmd = &cobra.Command{
	Use:   "keygen [VAULT]",
	Short: "Generate key material for a secrets vault whose backend supports it.",
	Long: "Generate key material in-process for a named secrets vault (a `secrets.providers.<name>` entry). " +
		"What gets generated is up to the vault's backend: a SOPS (age) vault produces an age key pair " +
		"(private identity → its key file; public recipient → `.sops.yaml`), and any backend that implements " +
		"the key-generation capability is dispatched the same way. Backends that do not support generation " +
		"report so and make no changes. With no VAULT argument, the single configured vault is used.",
	Args: cobra.MaximumNArgs(1),
	RunE: runSecretKeygen,
}

func init() {
	keygenParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Generate new material even if the vault already has some"),
	)
	keygenParser.RegisterFlags(keygenCmd)
}

// loadKeygenConfig loads the global atmos config for keygen. It is a seam so tests can inject a
// fixture configuration instead of reading atmos.yaml from disk. Keygen operates on the global
// `secrets.providers` config and needs no stack processing.
var loadKeygenConfig = func() (schema.AtmosConfiguration, error) {
	return cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
}

func runSecretKeygen(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretKeygen")()

	force, _ := cmd.Flags().GetBool("force")

	atmosConfig, err := loadKeygenConfig()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	vault, err := resolveKeygenVault(&atmosConfig, args)
	if err != nil {
		return err
	}

	kind := vaultKind(&atmosConfig, vault)
	prov, err := providers.New(&atmosConfig, trackForKind(kind), vault, nil)
	if err != nil {
		// A vault whose backend kind has no provider registered is, for keygen's purposes, simply
		// not supported — report it friendly rather than as a hard failure.
		if errors.Is(err, providers.ErrTrackNotRegistered) {
			notImplemented(vault, kind)
			return nil
		}
		return err
	}

	kg, ok := prov.(providers.KeyGenerator)
	if !ok {
		notImplemented(vault, kind)
		return nil
	}

	if kg.HasKey() && !force {
		ui.Infof("Vault `%s` already has key material. Use --force to generate more.", vault)
		return nil
	}

	res, err := kg.GenerateKey(atmosConfig.BasePath)
	if err != nil {
		// The backend implements keygen but cannot generate for this vault/kind — friendly.
		if errors.Is(err, providers.ErrKeygenNotSupported) {
			notImplemented(vault, kind)
			return nil
		}
		return err
	}

	printKeygenResult(res)
	return nil
}

// notImplemented prints the friendly "keygen not implemented for this backend" message.
func notImplemented(vault, kind string) {
	ui.Warningf("Key generation is not implemented for vault `%s` (kind %q). No changes made.", vault, kind)
}

// resolveKeygenVault picks the target vault: the explicit argument when given (validated against
// configured vaults), the only configured vault when exactly one exists, or an actionable error.
func resolveKeygenVault(atmosConfig *schema.AtmosConfiguration, args []string) (string, error) {
	vaults := configuredVaults(atmosConfig)

	if len(args) == 1 {
		name := args[0]
		for _, v := range vaults {
			if v == name {
				return name, nil
			}
		}
		return "", errUtils.Build(ErrNoVault).
			WithExplanationf("vault `%s` is not configured under `secrets.providers`", name).
			WithHintf("Configured vaults: %s", vaultsHint(vaults)).
			Err()
	}

	switch len(vaults) {
	case 0:
		return "", errUtils.Build(ErrNoVault).
			WithHint("Declare one under `secrets.providers` in atmos.yaml, e.g. `kind: sops/age` with a `spec.file`.").
			Err()
	case 1:
		return vaults[0], nil
	default:
		return "", errUtils.Build(ErrAmbiguousVault).
			WithHintf("Pass a vault name. Configured vaults: %s", vaultsHint(vaults)).
			Err()
	}
}

// configuredVaults returns the names of all `secrets.providers` vaults, sorted for stable output.
func configuredVaults(atmosConfig *schema.AtmosConfiguration) []string {
	names := make([]string, 0, len(atmosConfig.Secrets.Providers))
	for name := range atmosConfig.Secrets.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// vaultKind returns the configured kind for a vault (e.g. "sops/age").
func vaultKind(atmosConfig *schema.AtmosConfiguration, vault string) string {
	return atmosConfig.Secrets.Providers[vault].Kind
}

// trackForKind maps a provider kind to its registry track — the segment before the first "/"
// (e.g. "sops/age" → "sops", "ssl/x509" → "ssl"). Backends self-register under their track, so the
// command stays backend-agnostic.
func trackForKind(kind string) string {
	if i := strings.Index(kind, "/"); i >= 0 {
		return kind[:i]
	}
	return kind
}

func vaultsHint(vaults []string) string {
	if len(vaults) == 0 {
		return "(none)"
	}
	return strings.Join(vaults, ", ")
}

// printKeygenResult renders any provider's keygen output uniformly: human guidance on stderr, and
// the public material (if any) on stdout (safe to share / pipe).
func printKeygenResult(res *providers.KeygenResult) {
	ui.Successf("Vault `%s` (%s): %s", res.Vault, res.Kind, res.Summary)
	for _, out := range res.Outputs {
		note := "(safe to commit)"
		if out.Sensitive {
			note = "(keep out of version control)"
		}
		ui.Infof("%s → %s  %s", out.Label, out.Location, note)
	}
	if res.Public != "" {
		// Public material is not a secret — emit it on the data channel so it can be piped.
		_ = data.Writeln(res.Public)
	}
}
