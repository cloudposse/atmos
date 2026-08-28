package emulator

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cacerts"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	flociAzureCertPath = "tls/floci-az-selfsigned.crt"
	azureTrustBundle   = "floci-az-trust-bundle.pem"
	trustBundlePerm    = 0o644
)

func addAzureTrustEnv(profile *Profile, stack, name string) error {
	defer perf.Track(nil, "emulator.addAzureTrustEnv")()

	if profile == nil {
		return nil
	}
	dataDir := LookupInstanceDataDir(stack, name)
	if dataDir == "" {
		return nil
	}
	certPath := filepath.Join(dataDir, flociAzureCertPath)
	bundlePath, ok, err := buildAzureTrustBundle(certPath, filepath.Join(dataDir, azureTrustBundle))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if profile.Env == nil {
		profile.Env = map[string]string{}
	}
	profile.Env[cacerts.EnvSSLCertFile] = bundlePath
	return nil
}

func buildAzureTrustBundle(certPath, bundlePath string) (string, bool, error) {
	defer perf.Track(nil, "emulator.buildAzureTrustBundle")()

	certPEM, err := os.ReadFile(certPath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("%w: reading Azure emulator certificate: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	basePath := os.Getenv(cacerts.EnvSSLCertFile) //nolint:forbidigo // standard Go/OpenSSL trust override honored by Terraform/OpenTofu.
	if basePath == "" {
		basePath = cacerts.Find()
	}
	if basePath == "" {
		return "", false, nil
	}
	base, err := os.ReadFile(basePath) //nolint:gosec // basePath is an existing user/system CA bundle path.
	if err != nil {
		return "", false, fmt.Errorf("%w: reading system trust bundle: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	bundle := make([]byte, 0, len(base)+len(certPEM)+1)
	bundle = append(bundle, base...)
	bundle = append(bundle, '\n')
	bundle = append(bundle, certPEM...)
	if err := os.WriteFile(bundlePath, bundle, trustBundlePerm); err != nil { //nolint:gosec // bundlePath is derived from the emulator instance data directory.
		return "", false, fmt.Errorf("%w: writing Azure emulator trust bundle: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	return bundlePath, true, nil
}
