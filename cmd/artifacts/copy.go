package artifacts

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newCopyCommand() *cobra.Command {
	parser := newCopyParser()
	c := &command{action: artifact.ActionCopy, parser: parser, prefix: copyPrefix, run: runCopy, text: textCopy}
	cmd := &cobra.Command{
		Use:   "copy <ref>",
		Short: "Copy an artifact between two repositories",
		Long: `Copy an artifact from one configured repository to another. Identity, metadata, documents, and verified
content are preserved. Both the source and the destination verification policy are evaluated, and
--force never bypasses verification.`,
		Example: markdown.ArtifactsCopyUsageMarkdown,
		Args:    exactArgs("atmos artifacts copy <ref> --from=<alias> --to=<alias>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func runCopy(ctx context.Context, svc artifact.Service, in *input) (*result, error) {
	ref, err := in.ref()
	if err != nil {
		return nil, err
	}
	from, err := in.required(flagFrom, envFrom)
	if err != nil {
		return nil, err
	}
	to, err := in.required(flagTo, envTo)
	if err != nil {
		return nil, err
	}
	op, err := svc.Copy(ctx, artifact.CopyRequest{Ref: ref, From: from, To: to, Force: in.flag(flagForce)})
	return operationFrom(op), err
}

func textCopy(res *result) error {
	ui.Successf("Copied **%s** (%s).", res.Ref, res.Outcome)
	return textOperation(res)
}
