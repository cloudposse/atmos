package artifacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/ui"
)

// detailedPublisher is implemented by the real service. It returns the sealed Record next to the
// operation result, which a dry run needs in order to show the files it selected.
type detailedPublisher interface {
	PublishDetailed(ctx context.Context, request artifact.PublishRequest) (*service.PublishOutcome, error)
}

func newPublishCommand() *cobra.Command {
	parser := newPublishParser()
	c := &command{action: artifact.ActionPublish, parser: parser, prefix: publishPrefix, run: runPublish, text: textPublish}
	cmd := &cobra.Command{
		Use:   "publish <source>",
		Short: "Publish a file or directory as a versioned artifact",
		Long: `Publish a local file or directory to one configured repository. The artifact is addressed by name and
version; there is no implicit latest and no default repository. Republishing identical content is a
no-op, and different content at the same reference needs --force.`,
		Example: markdown.ArtifactsPublishUsageMarkdown,
		Args:    exactArgs("atmos artifacts publish <source> --repository=<alias> --name=<name> --version=<version>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func runPublish(ctx context.Context, svc artifact.Service, in *input) (*result, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	labels, err := in.labels()
	if err != nil {
		return nil, err
	}
	request := artifact.PublishRequest{
		Repository: repository,
		Source:     in.args[0],
		Name:       in.str(flagName),
		Version:    in.str(flagVersion),
		Kind:       in.str(flagKind),
		Metadata:   artifact.Metadata{Labels: labels},
		Readme:     in.str(flagReadme),
		Changelog:  in.str(flagChangelog),
		Force:      in.flag(flagForce),
		DryRun:     in.flag(flagDryRun),
	}
	if detailed, ok := svc.(detailedPublisher); ok {
		outcome, err := detailed.PublishDetailed(ctx, request)
		res := operationFrom(&outcome.Operation)
		if outcome.Record.Name != "" {
			record := outcome.Record
			res.Record = &record
		}
		return res, err
	}
	op, err := svc.Publish(ctx, request)
	return operationFrom(op), err
}

// textPublish renders a publication. A dry run lists the selected files, because the plan is the
// point of a dry run.
func textPublish(res *result) error {
	pairs := operationPairs(&res.OperationResult)
	if res.Outcome == artifact.OutcomeDryRun {
		ui.Infof("Dry run: nothing was written to repository **%s**.", res.Repository)
		if res.Record != nil {
			pairs = append(pairs, planPairs(res.Record)...)
		}
	} else {
		ui.Successf("Published **%s** to repository **%s** (%s).", res.Ref, res.Repository, res.Outcome)
	}
	return writeText(pairs)
}

// planPairs describe what a dry run would publish.
func planPairs(record *artifact.Record) []keyValue {
	pairs := []keyValue{{"kind", record.Kind}, {"description", record.Metadata.Description}}
	if len(record.Metadata.Labels) > 0 {
		pairs = append(pairs, keyValue{"labels", formatLabels(record.Metadata.Labels)})
	}
	var files []string
	for index := range record.Inventory {
		entry := record.Inventory[index]
		files = append(files, fmt.Sprintf("%s (%s)", entry.Path, entry.Type))
	}
	pairs = append(pairs, keyValue{"files", fmt.Sprintf("%d", len(files))})
	for _, document := range record.Documents {
		pairs = append(pairs, keyValue{string(document.Role), document.SourcePath})
	}
	for index, file := range files {
		pairs = append(pairs, keyValue{fmt.Sprintf("file[%d]", index), file})
	}
	return pairs
}

// formatLabels renders labels as sorted comma-separated key=value pairs.
func formatLabels(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for key, value := range labels {
		pairs = append(pairs, key+"="+value)
	}
	sortStrings(pairs)
	return strings.Join(pairs, ",")
}
