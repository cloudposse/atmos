package artifacts

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newCopyCommand() *cobra.Command {
	parser := newCopyParser()
	c := &command{action: artifact.ActionCopy, parser: parser, prefix: copyPrefix, params: paramsCopy, text: textCopy}
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

func paramsCopy(in *input) (*invoke.Params, error) {
	from, err := in.required(flagFrom, envFrom)
	if err != nil {
		return nil, err
	}
	to, err := in.required(flagTo, envTo)
	if err != nil {
		return nil, err
	}
	return &invoke.Params{Action: artifact.ActionCopy, Ref: in.args[0], From: from, To: to, Force: in.flag(flagForce)}, nil
}

func textCopy(res *result) error {
	ui.Successf("Copied **%s** (%s).", res.Ref, res.Outcome)
	return textOperation(res)
}
