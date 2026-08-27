package cloudformation

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// deliveryTimeout bounds an external target delivery (clone + commit + push).
const deliveryTimeout = 10 * time.Minute

// targetKey is the shared key for the `--target` flag and the summary entry that
// records the resolved provision target (mirrors helm/kubernetes' provision.go).
const targetKey = "target"

// kindAwsS3 is the provision-target kind for packaging destinations (template/
// asset uploads), matching the artifacts PRD's aws/s3 repository kind and the
// stores aws/ssm-style vocabulary.
const kindAwsS3 = "aws/s3"

// deliverApply resolves the selected provision target for an apply/deploy,
// packaging the template through a `kind: aws/s3` target first when needed, and
// delivers to it:
//   - the implicit/selected `kind: aws/cloudformation` target (the default when
//     no provision: is declared) deploys directly via the changeset flow.
//   - `kind: aws/s3` selected directly (--target <s3-target-name>) is
//     publish-only: upload the (optionally packaged) template and stop.
//   - any other kind (e.g. `kind: git`) packages through the aws/s3 target
//     first, then delivers the packaged reference via the generic target
//     registry (target.Deliver), the same producer-agnostic path Helm uses for
//     its own non-cluster deliveries.
func deliverApply(octx *opContext, client CloudFormationClient, spec *stackSpec) (map[string]any, *changeSetResult, error) {
	defer perf.Track(octx.AtmosConfig, "cloudformation.deliverApply")()

	summary := map[string]any{}
	provisionSection, _ := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	flagTarget, _ := octx.Flags[targetKey].(string)

	selected, err := target.SelectTargetWithDefault(provisionSection, flagTarget, "default", cfg.CloudFormationComponentType)
	if err != nil {
		return summary, nil, err
	}
	summary[targetKey] = selected.Name
	if summary[targetKey] == "" {
		summary[targetKey] = selected.Kind
	}

	if selected.Kind == cfg.CloudFormationComponentType {
		result, err := deployDirect(octx.Ctx, client, spec)
		return summary, result, err
	}

	if needsPackaging(spec.TemplateBody) || selected.Kind == kindAwsS3 {
		s3Target, err := resolvePackagingTarget(provisionSection, selected)
		if err != nil {
			return summary, nil, err
		}
		pkg, err := uploadPackage(octx.Ctx, octx.AtmosConfig, octx.Info, s3Target, spec.TemplateBody)
		if err != nil {
			return summary, nil, err
		}
		summary["package_url"] = pkg.URL
		summary["package_sha256"] = pkg.SHA256
	}

	if selected.Kind == kindAwsS3 {
		// Publish-only: template uploaded above, no deploy, no further delivery.
		return summary, nil, nil
	}

	// Any other kind (e.g. git): deliver the packaged reference generically.
	return summary, nil, deliverToExternalTarget(octx, selected, spec, summary)
}

// deployDirect executes the direct-deploy path: create (or reuse) a changeset
// and execute it, streaming live events while the stack converges.
func deployDirect(ctx context.Context, client CloudFormationClient, spec *stackSpec) (*changeSetResult, error) {
	result, err := createChangeSet(ctx, client, spec)
	if err != nil {
		return nil, err
	}
	if result.NoOp {
		return result, nil
	}

	if err := executeChangeSet(ctx, client, spec, result); err != nil {
		return result, err
	}

	status, err := streamStackEvents(ctx, client, spec.StackName)
	if err != nil {
		return result, err
	}
	if isFailedStackStatus(status) {
		return result, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationChangeSetFailed, spec.StackName, status)
	}
	return result, nil
}

// resolvePackagingTarget finds the `kind: aws/s3` provision target to package
// through. Resolution is implicit when the component declares exactly one
// `kind: aws/s3` target; with several, the deploy-style target must name its
// packaging store explicitly via `packaging: <target-name>` — an ambiguous
// setup is an error with a hint, never a silent guess.
func resolvePackagingTarget(provisionSection map[string]any, selected *target.SelectedTarget) (*targetS3Config, error) {
	if selected.Kind == kindAwsS3 {
		return s3ConfigFromTarget(selected.Name, selected.Config)
	}

	s3Targets := findS3Targets(provisionSection)
	switch len(s3Targets) {
	case 0:
		return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
			WithExplanation("Packaging is required (the template exceeds the inline size limit, or an aws/s3 target was selected) but no `kind: aws/s3` provision target is declared.").
			WithHint("Add a `provision.targets.<name>: {kind: aws/s3, bucket: ...}` entry.").
			Err()
	case 1:
		for name, cfgBlock := range s3Targets {
			return s3ConfigFromTarget(name, cfgBlock)
		}
	}

	if packagingName, ok := selected.Config["packaging"].(string); ok && packagingName != "" {
		if cfgBlock, found := s3Targets[packagingName]; found {
			return s3ConfigFromTarget(packagingName, cfgBlock)
		}
		return nil, fmt.Errorf("%w: packaging target %q not found among aws/s3 targets", errUtils.ErrInvalidAwsCloudFormationSettings, packagingName)
	}

	return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
		WithExplanationf("Multiple `kind: aws/s3` provision targets are declared; packaging is ambiguous.").
		WithHint("Set `packaging: <target-name>` on the deploy-style target to disambiguate.").
		Err()
}

// findS3Targets returns every `kind: aws/s3` entry in provision.targets.
func findS3Targets(provisionSection map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any)
	targets, _ := provisionSection["targets"].(map[string]any)
	for name, value := range targets {
		block, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := block["kind"].(string); kind == kindAwsS3 {
			result[name] = block
		}
	}
	return result
}

// s3ConfigFromTarget extracts bucket/prefix/region from a resolved `kind: aws/s3` target block.
func s3ConfigFromTarget(name string, block map[string]any) (*targetS3Config, error) {
	bucket, _ := block["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("%w: aws/s3 target %q is missing `bucket`", errUtils.ErrInvalidAwsCloudFormationSettings, name)
	}
	prefix, _ := block["prefix"].(string)
	region, _ := block["region"].(string)
	return &targetS3Config{Name: name, Bucket: bucket, Prefix: prefix, Region: region}, nil
}

// deliverToExternalTarget publishes the packaged template to a non-direct-deploy
// provision target (e.g. `kind: git`) as a producer-agnostic ProvisionArtifact
// via the target registry — the same registry-based delivery Helm uses for its
// own non-cluster targets, just carrying a CloudFormation template instead of
// Kubernetes manifests.
func deliverToExternalTarget(octx *opContext, selected *target.SelectedTarget, spec *stackSpec, summary map[string]any) error {
	fileName := spec.StackName + ".yaml"
	files := map[string][]byte{fileName: []byte(spec.TemplateBody)}
	summary["template_bytes"] = len(spec.TemplateBody)

	artifact := target.ProvisionArtifact{
		Kind:   target.ArtifactKindCloudFormationTemplate,
		Format: target.FormatYAML,
		Files:  files,
		Metadata: target.ArtifactMetadata{
			Component: octx.Info.ComponentFromArg,
			Stack:     octx.Info.Stack,
			Target:    selected.Name,
		},
	}

	deliverCtx, cancel := context.WithTimeout(octx.Ctx, deliveryTimeout)
	defer cancel()

	return target.Deliver(deliverCtx, selected.Kind, &target.DeliverInput{
		AtmosConfig:  octx.AtmosConfig,
		TargetName:   selected.Name,
		TargetConfig: selected.Config,
		Artifact:     artifact,
		EnvProvider:  authManagerFor(octx.Info),
	})
}

// authManagerFor returns the Atmos Auth manager as an identity-environment
// provider when configured, so targets that authenticate via Atmos Auth receive
// the composed environment (mirrors helm/provision.go's identical helper).
func authManagerFor(info *schema.ConfigAndStacksInfo) target.IdentityEnvironmentProvider {
	if mgr, ok := info.AuthManager.(auth.AuthManager); ok {
		return mgr
	}
	return nil
}
