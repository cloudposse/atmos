package verification

import (
	"context"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Verify verifies a downloaded asset according to request policy.
//
//nolint:gocritic // Request is intentionally passed by value for the public verifier API.
func (v *Verifier) Verify(ctx context.Context, req Request) (*Result, error) {
	defer perf.Track(nil, "verification.Verifier.Verify")()

	req.Policy = normalizePolicy(req.Policy)
	result := &Result{}
	if info, err := os.Stat(req.AssetPath); err == nil {
		result.AssetSize = info.Size()
	}

	if req.Policy.Checksums != PolicyDisabled {
		if err := v.verifyChecksum(ctx, &req, result); err != nil {
			return nil, err
		}
	} else {
		result.SkippedReasons = append(result.SkippedReasons, "checksum verification disabled")
	}

	if req.Policy.Signatures != PolicyDisabled {
		if err := v.verifySignatures(ctx, &req, result); err != nil {
			return nil, err
		}
	} else {
		result.SkippedReasons = append(result.SkippedReasons, "signature verification disabled")
	}

	return result, nil
}

func normalizePolicy(policy Policy) Policy {
	if policy.Checksums == "" {
		policy.Checksums = PolicyWhenAvailable
	}
	if policy.Signatures == "" {
		policy.Signatures = PolicyWhenAvailable
	}
	if policy.VerifierInstall == "" {
		policy.VerifierInstall = VerifierInstallAuto
	}
	return policy
}
