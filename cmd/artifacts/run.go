package artifacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/tags"
)

// input is the parsed state of one command invocation.
type input struct {
	v      *viper.Viper
	prefix string
	args   []string
}

func (in *input) key(name string) string { return in.prefix + "." + name }

func (in *input) str(name string) string { return in.v.GetString(in.key(name)) }

func (in *input) flag(name string) bool { return in.v.GetBool(in.key(name)) }

// repository returns the required repository alias, or a usage error naming the configured ones.
func (in *input) repository() (string, error) {
	alias := in.str(flagRepository)
	if alias != "" {
		return alias, nil
	}
	return "", errUtils.Build(fmt.Errorf("%w: --%s is required", errUtils.ErrRequiredFlagNotProvided, flagRepository)).
		WithHintf("Pass `--%s=<alias>` or set `%s`. Commands never pick a default repository.", flagRepository, envRepository).
		WithHint(configuredHint(configuredAliases(atmosConfigPtr))).
		WithExitCode(2).
		Err()
}

// required returns a flag that has no default and must be supplied.
func (in *input) required(name, env string) (string, error) {
	value := in.str(name)
	if value != "" {
		return value, nil
	}
	return "", errUtils.Build(fmt.Errorf("%w: --%s is required", errUtils.ErrRequiredFlagNotProvided, name)).
		WithHintf("Pass `--%s=<value>` or set `%s`.", name, env).
		WithExitCode(2).
		Err()
}

// labels parses the repeatable --labels flag with the shared exact key/value label parser.
func (in *input) labels() (map[string]string, error) {
	raw := tags.ReadLabelsFlagKey(in.v, in.key(flagLabels))
	return tags.ParseLabelsFlagFrom(strings.Join(raw, ","), "--labels (or "+envLabels+")")
}

// builder maps the parsed flags and arguments of one subcommand onto the transport-neutral
// operation parameters that the CLI, the artifact step, and hooks share.
type builder func(in *input) (*invoke.Params, error)

// command describes one result-producing subcommand.
type command struct {
	action artifact.Action
	parser *flags.StandardParser
	prefix string
	params builder
	text   func(*result) error
}

// execute is the shared pipeline: bind flags, validate the format, build the service, run the
// handler, render the result, and return the (decorated) error. A structured format emits its
// envelope even when the operation fails, and the returned error keeps the exit code nonzero.
func (c *command) execute(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "artifacts.execute")()

	values := viper.New()
	if err := c.parser.BindFlagsToViper(cmd, values); err != nil {
		return err
	}
	in := &input{v: values, prefix: c.prefix, args: args}
	format := in.str(flagFormat)
	if err := validateFormat(format, resultFormats); err != nil {
		return err
	}

	res, err := c.invoke(commandContext(cmd), in)
	if res == nil {
		res = &result{}
	}
	res.MarkFailed(err, c.action, in.str(flagRepository), requestedRef(args))
	if emitErr := emit(format, res, err != nil, c.text); emitErr != nil && err == nil {
		return emitErr
	}
	return decorate(err, configuredAliases(atmosConfigPtr))
}

// invoke builds the service, maps the flags onto operation parameters, and runs the operation
// through the shared implementation. The result is non-nil whenever the operation produced one,
// even alongside an error.
func (c *command) invoke(ctx context.Context, in *input) (*result, error) {
	if atmosConfigPtr == nil {
		return nil, errUtils.Build(errUtils.ErrArtifactConfigInvalid).
			WithExplanation("Atmos configuration is not loaded.").
			WithHint("Run this command from a project directory that contains an `atmos.yaml`, or pass `--config`.").
			Err()
	}
	svc, err := newService(atmosConfigPtr, ServiceOptions{AcceptTransparencyLog: in.v.GetBool(in.key(flagYes))})
	if err != nil {
		return nil, err
	}
	params, err := c.params(in)
	if err != nil {
		return nil, err
	}
	outcome, err := invoke.Run(ctx, svc, params)
	return outcome.Result, err
}

// commandContext returns the command's context, never nil.
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// requestedRef is the reference as the user typed it, for failure envelopes.
func requestedRef(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// exactArgs validates that exactly one positional argument is present, with an actionable message.
func exactArgs(usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == 1 {
			return nil
		}
		return errUtils.Build(fmt.Errorf("%w: expected 1 argument, got %d", errUtils.ErrInvalidPositionalArgs, len(args))).
			WithHintf("Usage: `%s`.", usage).
			WithExitCode(2).
			Err()
	}
}

// optionalArg validates a single optional positional argument.
func optionalArg(usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) <= 1 {
			return nil
		}
		return errUtils.Build(fmt.Errorf("%w: expected at most 1 argument, got %d", errUtils.ErrInvalidPositionalArgs, len(args))).
			WithHintf("Usage: `%s`.", usage).
			WithExitCode(2).
			Err()
	}
}
