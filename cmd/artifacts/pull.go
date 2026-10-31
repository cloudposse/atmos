package artifacts

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newPullCommand() *cobra.Command {
	parser := newPullParser()
	c := &command{action: artifact.ActionPull, parser: parser, prefix: pullPrefix, params: paramsPull, text: textPull}
	cmd := &cobra.Command{
		Use:   "pull <ref>",
		Short: "Verify and materialize an artifact into a directory",
		Long: `Download an artifact, verify every consumed descriptor (and its signature when policy requires it), and
materialize it at --output. The content is validated in staging first; an existing, different output
directory needs --force and is replaced, never overlaid.`,
		Example: markdown.ArtifactsPullUsageMarkdown,
		Args:    exactArgs("atmos artifacts pull <ref> --repository=<alias> --output=<dir>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func paramsPull(in *input) (*invoke.Params, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	destination, err := in.required(flagOutput, envOutput)
	if err != nil {
		return nil, err
	}
	return &invoke.Params{Action: artifact.ActionPull, Repository: repository, Ref: in.args[0], Destination: destination, Force: in.flag(flagForce)}, nil
}

func textPull(res *result) error {
	ui.Successf("Pulled **%s** from repository **%s**.", res.Ref, res.Repository)
	return textOperation(res)
}
