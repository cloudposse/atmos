package artifacts

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newSignCommand() *cobra.Command {
	parser := newSignParser()
	c := &command{action: artifact.ActionSign, parser: parser, prefix: signPrefix, params: paramsSign, text: textSign}
	cmd := &cobra.Command{
		Use:   "sign <digest-ref>",
		Short: "Add signature evidence to an immutable artifact digest",
		Long: `Sign the immutable subject of a digest reference (name@sha256:<digest>) and store the evidence beside
it. The digest may be the one the repository resolved (the OCI manifest digest, or the Record digest
for local and S3) or the Record digest; either way the portable Record is the signed subject.
Signing does not verify the artifact against trust policy: use atmos artifacts verify.`,
		Example: markdown.ArtifactsSignUsageMarkdown,
		Args:    exactArgs("atmos artifacts sign <name>@sha256:<digest> --repository=<alias>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func paramsSign(in *input) (*invoke.Params, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	return &invoke.Params{Action: artifact.ActionSign, Repository: repository, Ref: in.args[0]}, nil
}

func textSign(res *result) error {
	ui.Successf("Signed **%s** in repository **%s**. Run `atmos artifacts verify` to check it against your trust policy.", res.Requested, res.Repository)
	pairs := operationPairs(&res.OperationResult)
	if subject := res.Subject; subject != nil {
		pairs = append(pairs, keyValue{"subject", fmt.Sprintf("%s %s", subject.Kind, subject.Digest)})
	}
	for index := range res.Evidence {
		pairs = append(pairs, keyValue{fmt.Sprintf("evidence[%d]", index), string(res.Evidence[index].Descriptor.Digest)})
	}
	return writeText(pairs)
}
