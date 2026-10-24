//go:build darwin

package installer

import (
	"errors"
	"fmt"
	"os/exec"

	"golang.org/x/sys/unix"

	"github.com/cloudposse/atmos/pkg/perf"
)

// quarantineExtendedAttributes lists the macOS extended attributes that mark
// a file as an untrusted download. Modern macOS re-validates code-signing
// trust against these on every exec, not just on first Finder launch, so
// simply having downloaded-and-checksum-verified the binary is not enough to
// let it run.
var quarantineExtendedAttributes = []string{
	"com.apple.quarantine",
	"com.apple.provenance",
}

// trustVerifierBinary strips macOS download-provenance extended attributes
// from a bootstrap-installed verifier binary and ad-hoc re-signs it, mirroring
// what Homebrew does for downloaded formula binaries. Without this, freshly
// downloaded, ad-hoc/linker-signed release assets (e.g. cosign) are SIGKILLed
// by Gatekeeper/AMFI on every exec even though Atmos already verified their
// checksum. Only ever call this on binaries the installer itself just
// downloaded into its own bootstrap path — never on a binary discovered via
// exec.LookPath, which is user-managed and already OS-trusted.
func trustVerifierBinary(path string) error {
	defer perf.Track(nil, "installer.trustVerifierBinary")()

	if err := stripQuarantineAttributes(path); err != nil {
		return err
	}
	return adHocResignBinary(path)
}

func stripQuarantineAttributes(path string) error {
	for _, attr := range quarantineExtendedAttributes {
		if err := unix.Removexattr(path, attr); err != nil {
			if errors.Is(err, unix.ENOATTR) || errors.Is(err, unix.ENODATA) {
				continue // Attribute was never set; nothing to strip.
			}
			return fmt.Errorf("%w: remove %s from %s: %w", ErrVerifierTrustFailed, attr, path, err)
		}
	}
	return nil
}

func adHocResignBinary(path string) error {
	// #nosec G204 -- path is a binary Atmos itself just downloaded into its own
	// bootstrap directory and checksum-verified above; codesign is a
	// pre-installed macOS system utility, not a downloaded/managed tool.
	cmd := exec.Command("codesign", "--force", "--sign", "-", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: ad-hoc re-sign %s: %w\n%s", ErrVerifierTrustFailed, path, err, string(output))
	}
	return nil
}
