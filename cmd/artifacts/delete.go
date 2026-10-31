package artifacts

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newDeleteCommand() *cobra.Command {
	parser := newRepositoryParser(deletePrefix)
	c := &command{action: artifact.ActionDelete, parser: parser, prefix: deletePrefix, params: paramsDelete, text: textDelete}
	cmd := &cobra.Command{
		Use:   "delete <ref>",
		Short: "Delete one published artifact reference",
		Long: `Delete the selected published reference and the content it exclusively owns, where the repository
supports it. It never deletes a whole collection, purges other versions, or touches a catalog. The
command is noninteractive: name exactly the reference you mean.`,
		Example: markdown.ArtifactsDeleteUsageMarkdown,
		Args:    exactArgs("atmos artifacts delete <ref> --repository=<alias>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func paramsDelete(in *input) (*invoke.Params, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	return &invoke.Params{Action: artifact.ActionDelete, Repository: repository, Ref: in.args[0]}, nil
}

func textDelete(res *result) error {
	ui.Successf("Deleted **%s** from repository **%s**.", res.Requested, res.Repository)
	return textOperation(res)
}
