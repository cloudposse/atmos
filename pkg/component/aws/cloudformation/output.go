package cloudformation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	awscfn "github.com/cloudposse/atmos/pkg/aws/cloudformation"
	"github.com/cloudposse/atmos/pkg/data"
	sharedoutput "github.com/cloudposse/atmos/pkg/output"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// outputKeyFlag is the key under which the `output` verb's optional second
// positional argument (a single Output key) reaches the executor.
const outputKeyFlag = "key"

// describeStackOutputs fetches the deployed stack's Outputs via DescribeStacks,
// returning them as a plain map — the shape both the standalone `output` verb
// and the end-of-deploy summary render (via the shared pkg/output formatter).
func describeStackOutputs(ctx context.Context, client CloudFormationClient, stackName string) (map[string]any, error) {
	outputs, _, err := describeStackOutputValues(ctx, client, stackName)
	return outputs, err
}

// describeStackOutputValues retrieves raw outputs and deployed parameters together so presentation can
// determine sensitivity without changing dependency values.
func describeStackOutputValues(ctx context.Context, client CloudFormationClient, stackName string) (map[string]any, []cfntypes.Parameter, error) {
	defer perf.Track(nil, "cloudformation.describeStackOutputs")()

	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		if isStackNotFoundError(err) {
			return nil, nil, stackNotFoundError(stackName, err)
		}
		return nil, nil, wrapAPICallError(stackName, err)
	}
	if len(out.Stacks) == 0 {
		return map[string]any{}, nil, nil
	}
	// A stack that was never deployed (e.g. REVIEW_IN_PROGRESS from a preview
	// changeset) has no Outputs; reporting an empty map would look like success.
	if err := awscfn.CheckStackDeployed(stackName, out.Stacks[0].StackStatus); err != nil {
		return nil, nil, err
	}

	outputs := make(map[string]any, len(out.Stacks[0].Outputs))
	for _, o := range out.Stacks[0].Outputs {
		if o.OutputKey == nil {
			continue
		}
		outputs[*o.OutputKey] = stringValue(o.OutputValue)
	}
	return outputs, out.Stacks[0].Parameters, nil
}

// runOutput renders the deployed stack's Outputs via the standalone `output`
// verb's path (also called by runApply for the end-of-deploy summary). When the
// `key` flag is set, only that one Output is rendered.
func runOutput(ctx context.Context, client CloudFormationClient, stackName string, flags map[string]any, summary map[string]any) (map[string]any, error) {
	outputs, err := presentedStackOutputs(ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["outputs"] = outputs

	format, err := outputFormat(flags)
	if err != nil {
		return summary, err
	}

	if key, _ := flags[outputKeyFlag].(string); key != "" {
		return summary, renderSingleOutput(outputs, stackName, key, format, flags)
	}
	if len(outputs) == 0 && format == sharedoutput.FormatTable {
		// An empty Key/Value table is noise; structured formats still render an
		// empty document so pipelines keep getting parseable stdout.
		ui.Infof("Stack %s has no outputs", stackName)
		return summary, nil
	}
	if err := renderOutputsSummary(outputs, flags); err != nil {
		return summary, err
	}
	return summary, nil
}

// outputFormat resolves and validates the --format flag (default: table). An
// unsupported value is an error that names the value and lists the valid ones.
func outputFormat(flags map[string]any) (sharedoutput.Format, error) {
	format := sharedoutput.FormatTable
	f, ok := flags["format"].(string)
	if !ok || f == "" {
		return format, nil
	}
	for _, supported := range sharedoutput.SupportedFormats {
		if f == supported {
			return sharedoutput.Format(f), nil
		}
	}
	return "", fmt.Errorf("%w: failed to format outputs: unsupported --format %q (valid: %s)", errUtils.ErrInvalidFlag, f, strings.Join(sharedoutput.SupportedFormats, ", "))
}

// outputFormatOptions builds the shared formatter options from the flatten and
// uppercase flags.
func outputFormatOptions(flags map[string]any) sharedoutput.FormatOptions {
	opts := sharedoutput.FormatOptions{}
	if flatten, ok := flags["flatten"].(bool); ok {
		opts.Flatten = flatten
	}
	if uppercase, ok := flags["uppercase"].(bool); ok {
		opts.Uppercase = uppercase
	}
	return opts
}

// renderOutputsSummary writes the Outputs to the data channel (stdout) in the
// requested format (default: table), reusing the shared pkg/output formatter —
// the full standard format set (json/yaml/hcl/env/dotenv/bash/csv/tsv/github).
// Returns an error on an invalid --format (wrapped in ErrInvalidFlag, instead of
// swallowing it: a bad value must fail the command, not silently exit 0 with
// empty stdout) or on a write failure, so callers report the operation as
// unsuccessful instead of silently succeeding with no output.
func renderOutputsSummary(outputs map[string]any, flags map[string]any) error {
	format, err := outputFormat(flags)
	if err != nil {
		return err
	}
	opts := outputFormatOptions(flags)

	if format == sharedoutput.FormatJSON {
		rendered, err := formatOutputsJSON(outputs, opts)
		if err != nil {
			return err
		}
		return data.Write(rendered)
	}

	rendered, err := sharedoutput.FormatOutputsWithOptions(outputs, format, opts)
	if err != nil {
		return fmt.Errorf("%w: failed to format outputs: %w", errUtils.ErrInvalidFlag, err)
	}
	return data.Write(rendered)
}

// renderSingleOutput writes one Output to stdout. The default (table) format
// prints the bare value so it is pipeable (`$(atmos aws cloudformation output
// vpc VpcId -s dev)`); json encodes the value; every other format goes through
// the shared single-value formatter (e.g. env prints KEY=value).
func renderSingleOutput(outputs map[string]any, stackName, key string, format sharedoutput.Format, flags map[string]any) error {
	value, err := awscfn.LookupOutput(outputs, stackName, key)
	if err != nil {
		return err
	}

	switch format {
	case sharedoutput.FormatTable:
		return data.Writeln(fmt.Sprint(value))
	case sharedoutput.FormatJSON:
		rendered, err := encodeJSON(value)
		if err != nil {
			return err
		}
		return data.Write(rendered)
	default:
		rendered, err := sharedoutput.FormatSingleValueWithOptions(key, value, format, outputFormatOptions(flags))
		if err != nil {
			return fmt.Errorf("%w: failed to format output %q: %w", errUtils.ErrInvalidFlag, key, err)
		}
		return data.Write(rendered)
	}
}

// formatOutputsJSON renders the Outputs as an indented JSON object without HTML
// escaping, applying the flatten and uppercase options like the shared formatter.
func formatOutputsJSON(outputs map[string]any, opts sharedoutput.FormatOptions) (string, error) {
	transformed := outputs
	if opts.Flatten {
		transformed = sharedoutput.FlattenMap(transformed, "", opts.GetFlattenSeparator())
	}
	if opts.Uppercase {
		upper := make(map[string]any, len(transformed))
		for k, v := range transformed {
			upper[strings.ToUpper(k)] = v
		}
		transformed = upper
	}
	return encodeJSON(transformed)
}

// encodeJSON marshals value as indented JSON with HTML escaping disabled.
// The standard encoder escapes angle brackets and ampersands by default, which
// would render the masking placeholder as an escaped sequence in CLI output
// meant for people and shell pipelines.
func encodeJSON(value any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return "", fmt.Errorf("failed to marshal outputs to JSON: %w", err)
	}
	return buf.String(), nil
}
