package artifacts

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
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
		run: func(ctx context.Context, svc artifact.Service, in *input) (*result, error) {
			return runShow(ctx, svc, in, role)
		},
		text: textDocument,
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

func runShow(ctx context.Context, svc artifact.Service, in *input, role artifact.DocumentRole) (*result, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	ref, err := in.ref()
	if err != nil {
		return nil, err
	}
	document, err := svc.ShowDocument(ctx, artifact.ShowDocumentRequest{Repository: repository, Ref: ref, Role: role})
	if document == nil {
		return nil, err
	}
	res := operationFrom(&document.Operation)
	if document.Document.Digest != "" {
		descriptor := document.Document
		res.Document = &descriptor
		content := string(document.Content)
		res.Content = &content
	}
	return res, err
}

// textDocument prints the document itself, so it can be piped or redirected as-is.
func textDocument(res *result) error {
	if res.Content == nil {
		return nil
	}
	return data.Write(*res.Content)
}
