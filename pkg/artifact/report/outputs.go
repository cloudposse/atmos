package report

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/ci"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
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
// The location has its credentials and query string removed first, and every value is masked: a CI
// output variable is readable by every later step and job.
//
// Step names that differ but normalize to the same segment (publish-vpc and publish_vpc) can write
// the same qualified name. A qualified name is claimed by the first step that actually writes it,
// in report order; a later step with a different name writes only the unqualified names for that
// output and a warning says so, so a qualified name never carries another step's value. Steps that
// write disjoint outputs (for example a delete and a publish) do not conflict.
func writeOutputs(reporter ci.Reporter, entries []*Entry) error {
	claims := newNameClaims()
	for _, entry := range entries {
		if entry.Outcome == nil || entry.Outcome.Result == nil {
			continue
		}
		segment := outputStepName(entry.Step)
		for _, pair := range invoke.OutputPairs(&entry.Outcome.Result.OperationResult) {
			value := outputValue(pair)
			for _, name := range outputNames(claims.qualifier(entry.Step, segment, pair.Name), pair.Name) {
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

// nameClaims remembers which step name owns each step-qualified output name.
type nameClaims struct {
	owners map[string]string
	warned map[string]bool
}

func newNameClaims() *nameClaims {
	return &nameClaims{owners: map[string]string{}, warned: map[string]bool{}}
}

// qualifier returns the segment to qualify one output of a step with, or "" when the step has no
// usable segment or another step name already owns the qualified name of this output.
func (c *nameClaims) qualifier(step, segment, name string) string {
	if segment == "" {
		return ""
	}
	qualified := OutputPrefix + segment + "_" + name
	owner, claimed := c.owners[qualified]
	if !claimed {
		c.owners[qualified] = step
		return segment
	}
	if owner == step {
		return segment
	}
	if !c.warned[step] {
		c.warned[step] = true
		log.Warn("Artifact step output names collide; step-qualified outputs are skipped for the later step",
			"step", atmosio.MaskString(step), "collides_with", atmosio.MaskString(owner),
			"output_prefix", atmosio.MaskString(OutputPrefix+segment+"_"))
	}
	return ""
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
// query strings first; every value then passes through the secret masker, because a registered
// secret can appear in a reference or in a location path as well.
func outputValue(pair invoke.Pair) string {
	if pair.Name == invoke.OutputLocation {
		return atmosio.MaskString(redactURLs(pair.Value))
	}
	return atmosio.MaskString(pair.Value)
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
