package cloudformation

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Summary keys describing where a published template went, both recorded by
// packaging (packageS3URIKey is derived from the https URL when absent).
const (
	packageURLKey   = "package_url"
	packageS3URIKey = "package_s3_uri"
)

// confirmApply asks the user to approve the previewed changes, unless
// --auto-approve was passed. A decline is ErrUserAborted with an explanation of
// what did (not) happen, so the abort is never mistaken for a failure.
func confirmApply(octx *opContext, stackName string) error {
	err := requireConfirmation(OperationApply, stackName, octx.Flags)
	if err == nil || !errors.Is(err, errUtils.ErrUserAborted) {
		return err
	}
	return errUtils.Build(errUtils.ErrUserAborted).
		WithExplanationf("Apply of stack %q was declined at the confirmation prompt. No changes were made, and the preview changeset was discarded.", stackName).
		Err()
}

// announceNoChanges reports a no-op apply: the stack already matches the desired
// state. The caller deletes the FAILED "didn't contain changes" changeset.
func announceNoChanges(stackName string) {
	ui.Info(fmt.Sprintf("No changes: stack %s already matches the desired state", stackName))
}

// renderApplyPreview prints the predicted changes on the UI channel (stderr),
// ahead of the confirmation prompt. It is human-facing context for the prompt,
// so it must not pollute stdout, which carries the stack Outputs (for example
// with --format=json).
func renderApplyPreview(stackName string, result *changeSetResult) {
	ui.Writeln(diffSummaryText(stackName, result))
}

// stackSetTargetError explains that apply/deploy cannot deliver to a
// `kind: aws/stackset` target and points at the StackSet verbs that can.
func stackSetTargetError(targetName string, spec *stackSpec) error {
	hintTarget := ""
	if targetName != "" {
		hintTarget = " --target " + targetName
	}
	return errUtils.Build(errUtils.ErrAwsCloudFormationStackSetTargetNotApplicable).
		WithExplanationf("Provision target %q is a `kind: aws/stackset` target. apply and deploy deliver a single stack; StackSets are managed by their own verbs.", targetName).
		WithHintf("Run `atmos aws cloudformation stackset create %s%s` to create it, or `stackset update` to change an existing StackSet.", spec.commandTarget(), hintTarget).
		Err()
}

// reportPublished prints what a publish-only apply (an `aws/s3` target selected
// directly) did: the template went to S3 and no stack was touched. It records the
// s3:// location next to the https TemplateURL in the summary.
func reportPublished(selected *target.SelectedTarget, summary map[string]any) {
	httpsURL, _ := summary[packageURLKey].(string)
	// Packaging records the s3:// location itself; recover it from the https URL
	// only when it did not.
	s3URI, _ := summary[packageS3URIKey].(string)
	hasURI := s3URI != ""
	if !hasURI {
		s3URI, hasURI = s3URIFromPackageURL(httpsURL)
	}
	if hasURI {
		summary[packageS3URIKey] = s3URI
	}
	switch {
	case hasURI:
		ui.Success(fmt.Sprintf("Published template to %s (TemplateURL: %s)", s3URI, httpsURL))
	case httpsURL != "":
		ui.Success(fmt.Sprintf("Published template (TemplateURL: %s)", httpsURL))
	default:
		ui.Success(fmt.Sprintf("Published template to aws/s3 target %q", selected.Name))
	}
}

// s3URIFromPackageURL recovers the s3://bucket/key location from the https URL
// packaging builds (virtual-hosted style https://bucket.s3.region.amazonaws.com/key,
// or path style https://s3.region.amazonaws.com/bucket/key for dotted bucket
// names). It reports false when raw is empty or not one of those shapes.
func s3URIFromPackageURL(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	host := u.Hostname()
	key := strings.TrimPrefix(u.Path, "/")
	if strings.HasPrefix(host, "s3.") {
		bucket, rest, ok := strings.Cut(key, "/")
		if !ok || bucket == "" || rest == "" {
			return "", false
		}
		return "s3://" + bucket + "/" + rest, true
	}
	bucket, _, found := strings.Cut(host, ".s3.")
	if !found || bucket == "" || key == "" {
		return "", false
	}
	return "s3://" + bucket + "/" + key, true
}

// reportDelivered prints what an external-target delivery (for example
// `kind: git`) did, naming the target and the delivered file.
func reportDelivered(selected *target.SelectedTarget, fileName string, summary map[string]any) {
	summary["delivered_file"] = fileName
	name := selected.Name
	if name == "" {
		name = selected.Kind
	}
	ui.Success(fmt.Sprintf("Delivered %s to %s target %q", fileName, selected.Kind, name))
}
