package artifacts

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/perf"
)

// KindPublishSpec describes a convenience publish command that supplies its artifact kind, such as
// `atmos scaffold publish`. The command delegates to the same publish pipeline as
// `atmos artifacts publish`: same flags (without --kind), same environment variables, same service,
// same result rendering, so the two can never drift.
type KindPublishSpec struct {
	// Use is the command usage line, for example "publish <source>".
	Use string

	// Short is the one-line description.
	Short string

	// Long is the long description.
	Long string

	// Example is the rendered usage examples.
	Example string

	// Kind is the artifact kind the command publishes, for example "scaffold".
	Kind string

	// Prefix is the Viper key prefix that keeps this command's values apart from every other
	// command's, for example "scaffold.publish".
	Prefix string

	// Usage is the argument usage shown when the source argument is missing.
	Usage string
}

// NewKindPublishCommand builds a convenience publish command for one artifact kind.
func NewKindPublishCommand(spec *KindPublishSpec) *cobra.Command {
	defer perf.Track(nil, "artifacts.NewKindPublishCommand")()

	parser := newPublishParserFor(spec.Prefix, false)
	c := &command{action: artifact.ActionPublish, parser: parser, prefix: spec.Prefix, params: kindPublishParams(spec.Kind), text: textPublish}
	cmd := &cobra.Command{
		Use:     spec.Use,
		Short:   spec.Short,
		Long:    spec.Long,
		Example: spec.Example,
		Args:    exactArgs(spec.Usage),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

// kindPublishParams maps the publish flags like `atmos artifacts publish` does and then supplies the
// command's own kind.
func kindPublishParams(kind string) builder {
	return func(in *input) (*invoke.Params, error) {
		params, err := paramsPublish(in)
		if err != nil {
			return nil, err
		}
		params.Kind = kind
		return params, nil
	}
}
