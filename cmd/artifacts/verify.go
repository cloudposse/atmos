package artifacts

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newVerifyCommand() *cobra.Command {
	parser := newRepositoryParser(verifyPrefix)
	c := &command{action: artifact.ActionVerify, parser: parser, prefix: verifyPrefix, params: paramsVerify, text: textVerify}
	cmd := &cobra.Command{
		Use:   "verify <ref>",
		Short: "Require a trusted, valid signature for an artifact",
		Long: `Verify the stored signature evidence of an artifact against the effective policy: the global
artifacts.verification trust set and the repository's own, both of which must be satisfied. Verify
always requires a trusted valid signature. Without trust configured it fails with an unchecked result
instead of reporting success, and the exit code is nonzero on any failure.`,
		Example: markdown.ArtifactsVerifyUsageMarkdown,
		Args:    exactArgs("atmos artifacts verify <ref> --repository=<alias>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func paramsVerify(in *input) (*invoke.Params, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	return &invoke.Params{Action: artifact.ActionVerify, Repository: repository, Ref: in.args[0]}, nil
}

func textVerify(res *result) error {
	ui.Successf("Verified **%s** in repository **%s**.", res.Requested, res.Repository)
	pairs := operationPairs(&res.OperationResult)
	pairs = append(pairs, keyValue{"verification", verificationState(res.Verification)})
	if subject := res.Subject; subject != nil {
		pairs = append(pairs, keyValue{"subject", fmt.Sprintf("%s %s", subject.Kind, subject.Digest)})
	}
	if verification := res.Verification; verification != nil {
		pairs = append(pairs, keyValue{"policy_fingerprint", verification.PolicyFingerprint})
		for index, claim := range verification.Claims {
			pairs = append(pairs, keyValue{fmt.Sprintf("signer[%d]", index), describeClaim(claim)})
		}
	}
	return writeText(pairs)
}

// describeClaim renders a verified signer claim.
func describeClaim(claim artifact.SignerClaim) string {
	switch {
	case claim.Identity != "":
		return fmt.Sprintf("%s (issuer %s)", claim.Identity, claim.Issuer)
	case claim.KeyID != "":
		return "key " + claim.KeyID
	default:
		return "unknown signer"
	}
}
