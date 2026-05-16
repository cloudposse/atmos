package verification

import (
	"context"
	"errors"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

const (
	PolicyWhenAvailable = "when_available"
	PolicyRequired      = "required"
	PolicyDisabled      = "disabled"

	VerifierInstallAuto     = "auto"
	VerifierInstallPathOnly = "path_only"
)

var (
	ErrChecksumRequired        = errors.New("checksum verification required")
	ErrChecksumMismatch        = errors.New("checksum mismatch")
	ErrChecksumNotFound        = errors.New("checksum not found")
	ErrDownloadFailed          = errors.New("verification sidecar download failed")
	ErrUnsupportedAlgorithm    = errors.New("unsupported checksum algorithm")
	ErrSignatureRequired       = errors.New("signature verification required")
	ErrSignatureFailed         = errors.New("signature verification failed")
	ErrVerifierCommandRequired = errors.New("verifier command required")
)

// Downloader fetches verification sidecar files such as checksum files and signatures.
type Downloader interface {
	Download(ctx context.Context, url string) ([]byte, error)
}

// CommandRunner runs external verification commands.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

// Request contains all data required to verify a downloaded tool asset.
type Request struct {
	Tool       *registry.Tool
	Version    string
	AssetURL   string
	AssetPath  string
	Policy     Policy
	Downloader Downloader
	Runner     CommandRunner
}

// Policy controls verification behavior.
type Policy struct {
	Checksums       string
	Signatures      string
	VerifierInstall string
}

// Result describes verification that was performed or skipped.
type Result struct {
	AssetSize         int64
	ChecksumAlgorithm string
	Checksum          string
	SignatureMethods  []string
	SkippedReasons    []string
}

// Verifier verifies downloaded tool assets.
type Verifier struct {
	Downloader Downloader
	Runner     CommandRunner
}

// PolicyFromConfig returns a fully defaulted verification policy.
func PolicyFromConfig(config *schema.ToolchainVerification) Policy {
	defer perf.Track(nil, "verification.PolicyFromConfig")()

	if config == nil {
		return Policy{
			Checksums:       PolicyWhenAvailable,
			Signatures:      PolicyWhenAvailable,
			VerifierInstall: VerifierInstallAuto,
		}
	}

	return Policy{
		Checksums:       defaultPolicy(config.Checksums),
		Signatures:      defaultPolicy(config.Signatures),
		VerifierInstall: defaultVerifierInstall(config.VerifierInstall),
	}
}

func defaultPolicy(value string) string {
	switch value {
	case PolicyRequired, PolicyDisabled, PolicyWhenAvailable:
		return value
	default:
		return PolicyWhenAvailable
	}
}

func defaultVerifierInstall(value string) string {
	switch value {
	case VerifierInstallPathOnly, VerifierInstallAuto:
		return value
	default:
		return VerifierInstallAuto
	}
}
