package cache

import (
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui"
)

var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust the cache proxy certificate in the OS trust store",
	Long: `Install the registry cache's self-signed certificate into the OS trust store so
terraform/tofu trust the HTTPS cache proxy.

On Linux, Atmos trusts the certificate automatically via SSL_CERT_FILE, so this is
not needed. On macOS and Windows, Go ignores SSL_CERT_FILE and uses the platform
verifier, so a one-time trust step is required (you may be prompted for your
password).`,
	Example: `  atmos terraform cache trust`,
	Args:    cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		defer perf.Track(atmosConfigPtr, "cache.trust.RunE")()

		certPath, err := cacheCertPath()
		if err != nil {
			return err
		}

		required, note := tfcache.TrustInstructions()
		if !required {
			ui.Info(note)
			return nil
		}
		if err := tfcache.InstallTrust(certPath); err != nil {
			return err
		}
		ui.Success("Trusted the Terraform registry cache certificate")
		return nil
	},
}

var untrustCmd = &cobra.Command{
	Use:     "untrust",
	Short:   "Remove the cache proxy certificate from the OS trust store",
	Long:    `Remove the registry cache's self-signed certificate from the OS trust store.`,
	Example: `  atmos terraform cache untrust`,
	Args:    cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		defer perf.Track(atmosConfigPtr, "cache.untrust.RunE")()

		certPath, err := cacheCertPath()
		if err != nil {
			return err
		}

		required, note := tfcache.TrustInstructions()
		if !required {
			ui.Info(note)
			return nil
		}
		if err := tfcache.RemoveTrust(certPath); err != nil {
			return err
		}
		ui.Success("Removed the Terraform registry cache certificate from the trust store")
		return nil
	},
}

// cacheCertPath resolves the proxy certificate path from the Atmos configuration.
func cacheCertPath() (string, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return "", err
	}
	return tfcache.ProxyCertPath(&atmosConfig)
}
