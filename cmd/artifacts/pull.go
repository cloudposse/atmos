package artifacts

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newPullCommand() *cobra.Command {
	parser := newPullParser()
	c := &command{action: artifact.ActionPull, parser: parser, prefix: pullPrefix, run: runPull, text: textPull}
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

func runPull(ctx context.Context, svc artifact.Service, in *input) (*result, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	ref, err := in.ref()
	if err != nil {
		return nil, err
	}
	destination, err := in.required(flagOutput, envOutput)
	if err != nil {
		return nil, err
	}
	op, err := svc.Pull(ctx, artifact.PullRequest{Repository: repository, Ref: ref, Destination: destination, Force: in.flag(flagForce)})
	return operationFrom(op), err
}

func textPull(res *result) error {
	ui.Successf("Pulled **%s** from repository **%s**.", res.Ref, res.Repository)
	return textOperation(res)
}
