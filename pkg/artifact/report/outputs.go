package report

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ci"
)

// OutputPrefix namespaces the CI output variables of artifact operations, so a job that publishes
// an artifact and also runs Terraform never sees two features write the same name.
const OutputPrefix = "artifact_"

// writeOutputs writes the named outputs of every single-artifact operation to the CI output
// variables, in report order. Each operation writes artifact_<name> (outcome, ref, location,
// resolved_digest, record_digest, content_digest, representation_digest, size), the same names a
// step exposes as StepResult.Outputs, so the last operation in the report owns the unqualified
// names. An operation that ran in a named step also writes artifact_<step>_<name>, which stays
// unambiguous when a job runs several operations. List and search have no named outputs.
//
// The location has its credentials and query string removed first: a CI output variable is
// readable by every later step and job.
func writeOutputs(reporter ci.Reporter, entries []*Entry) error {
	for _, entry := range entries {
		if entry.Outcome == nil || entry.Outcome.Result == nil {
			continue
		}
		step := outputStepName(entry.Step)
		for _, pair := range invoke.OutputPairs(&entry.Outcome.Result.OperationResult) {
			value := outputValue(pair)
			for _, name := range outputNames(step, pair.Name) {
				// A provider that cannot write one output cannot write the rest, so the first
				// failure ends the attempt instead of repeating the same error for every variable.
				if _, err := reporter.Output(name, value); err != nil {
					return fmt.Errorf("%w: %w", errUtils.ErrArtifactReportWrite, err)
				}
			}
		}
	}
	return nil
}

// outputNames returns the output variable names for one value: the unqualified name and, for an
// operation in a named step, the step-qualified name.
func outputNames(step, name string) []string {
	names := []string{OutputPrefix + name}
	if step != "" {
		names = append(names, OutputPrefix+step+"_"+name)
	}
	return names
}

// outputValue is the value written for a named output. The location is stripped of credentials and
// query strings; the other values are digests, sizes, and references.
func outputValue(pair invoke.Pair) string {
	if pair.Name == invoke.OutputLocation {
		return redactURLs(pair.Value)
	}
	return pair.Value
}

// outputStepName turns a step name into a safe output-name segment: lowercase letters, digits,
// and underscores. A name with no usable characters yields an empty segment.
func outputStepName(step string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(step) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}
