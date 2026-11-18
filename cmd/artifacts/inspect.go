package artifacts

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact"
)

func newInspectCommand() *cobra.Command {
	parser := newRepositoryParser(inspectPrefix)
	c := &command{action: artifact.ActionInspect, parser: parser, prefix: inspectPrefix, run: runInspect, text: textInspect}
	cmd := &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Show the metadata of an artifact without downloading its payload",
		Long: `Read an artifact's metadata: identity, kind, labels, documents, storage representation, and signature
evidence. Content and Record digests are labeled separately from the OCI manifest or archive
checksums. Without trust configuration the artifact is reported as unchecked, never verified.`,
		Example: markdown.ArtifactsInspectUsageMarkdown,
		Args:    exactArgs("atmos artifacts inspect <ref> --repository=<alias>"),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

func runInspect(ctx context.Context, svc artifact.Service, in *input) (*result, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	ref, err := in.ref()
	if err != nil {
		return nil, err
	}
	inspected, err := svc.Inspect(ctx, artifact.InspectRequest{Repository: repository, Ref: ref})
	if inspected == nil {
		return nil, err
	}
	res := operationFrom(&inspected.Operation)
	res.Inspection = &inspected.Inspection
	res.Verification = inspected.Verification
	return res, err
}

func textInspect(res *result) error {
	pairs := operationPairs(&res.OperationResult)
	if inspection := res.Inspection; inspection != nil {
		pairs = append(pairs, inspectionPairs(inspection)...)
	}
	pairs = append(pairs, keyValue{"verification", verificationState(res.Verification)})
	return writeText(pairs)
}

// inspectionPairs describe the Record and the storage representation of an inspected artifact.
func inspectionPairs(inspection *artifact.Inspection) []keyValue {
	var pairs []keyValue
	if record := inspection.Record; record != nil {
		pairs = append(
			pairs,
			keyValue{"name", record.Name},
			keyValue{"version", record.Version},
			keyValue{"kind", record.Kind},
			keyValue{"created_at", record.CreatedAt.String()},
			keyValue{"description", record.Metadata.Description},
			keyValue{"labels", formatLabels(record.Metadata.Labels)},
			keyValue{"files", fmt.Sprintf("%d", len(record.Inventory))},
		)
		for _, document := range record.Documents {
			pairs = append(pairs, keyValue{string(document.Role), fmt.Sprintf("%s (%d bytes)", document.SourcePath, document.Size)})
		}
	}
	pairs = append(pairs, keyValue{"packaging", string(inspection.Representation.Packaging)})
	if root := inspection.Representation.OriginalRoot; root != nil {
		pairs = append(pairs, keyValue{"original_root", string(root.Digest)})
	}
	if count := len(inspection.Evidence); count > 0 {
		pairs = append(pairs, keyValue{"signatures", fmt.Sprintf("%d (untrusted until verified)", count)})
	}
	return pairs
}

// verificationState reports the policy outcome. Absent means unchecked: nothing was verified.
func verificationState(verification *artifact.VerificationResult) string {
	if verification == nil {
		return string(artifact.VerificationUnchecked)
	}
	return string(verification.Status)
}
