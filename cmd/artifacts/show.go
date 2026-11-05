package artifacts

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/data"
)

func newShowCommand() *cobra.Command {
	show := &cobra.Command{
		Use:     "show",
		Short:   "Show a package document (README or CHANGELOG) without downloading the payload",
		Long:    "Read the README or CHANGELOG of an artifact from its stored Record. The payload is never downloaded.",
		Example: markdown.ArtifactsShowUsageMarkdown,
	}
	show.AddCommand(newDocumentCommand(artifact.DocumentReadme), newDocumentCommand(artifact.DocumentChangelog))
	return show
}

func newDocumentCommand(role artifact.DocumentRole) *cobra.Command {
	parser := newRepositoryParser(showPrefix + "." + string(role))
	c := &command{
		action: artifact.ActionShow, parser: parser, prefix: showPrefix + "." + string(role),
		params: func(in *input) (*invoke.Params, error) { return paramsShow(in, role) },
		text:   textDocument,
	}
	cmd := &cobra.Command{
		Use:   string(role) + " <ref>",
		Short: "Show the " + string(role) + " of an artifact",
		Long: "Read the " + string(role) + " of an artifact from its stored Record and print it to stdout. Only the\n" +
			"document is read; the payload is never downloaded. A missing document is an error.",
		Example: markdown.ArtifactsShowUsageMarkdown,
		Args:    exactArgs("atmos artifacts show " + string(role) + " <ref> --repository=<alias>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func paramsShow(in *input, role artifact.DocumentRole) (*invoke.Params, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	return &invoke.Params{Action: artifact.ActionShow, Repository: repository, Ref: in.args[0], Document: string(role)}, nil
}

// textDocument prints the document itself, so it can be piped or redirected as-is.
func textDocument(res *result) error {
	if res.Content == nil {
		return nil
	}
	return data.Write(*res.Content)
}
