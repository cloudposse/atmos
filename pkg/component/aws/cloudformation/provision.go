package cloudformation

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
//     no provision: is declared) deploys directly via the changeset flow:
//     changeset, preview, confirmation, execute.
//   - `kind: aws/s3` selected directly (--target <s3-target-name>) is
//     publish-only: upload the (optionally packaged) template, report where it
//     went, and stop. No stack changes, so no stack-change confirmation.
//   - `kind: aws/stackset` is rejected with a pointer to the stackset verbs.
//   - any other kind (e.g. `kind: git`) packages through the aws/s3 target
//     first, then delivers the packaged reference via the generic target
//     registry (target.Deliver), the same producer-agnostic path Helm uses for
//     its own non-cluster deliveries. It also asks no stack-change question.
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

	if selected.Kind == kindAwsStackSet {
		return summary, nil, stackSetTargetError(selected.Name)
	}

	// Only a direct stack deploy asks the stack-change question. Fail fast, before
	// packaging uploads anything or a changeset exists, when it cannot be asked.
	direct := selected.Kind == cfg.CloudFormationComponentType
	if direct {
		if err := requireInteractiveOrAutoApprove(OperationApply, octx.Flags); err != nil {
			return summary, nil, err
		}
	}

	if err := packageIfNeeded(octx, provisionSection, selected, spec, summary); err != nil {
		return summary, nil, err
	}

	if direct {
		result, err := deployDirect(octx, client, spec)
		return summary, result, err
	}

	if selected.Kind == kindAwsS3 {
		// Publish-only: template uploaded above, no deploy, no further delivery.
		reportPublished(selected, summary)
		return summary, nil, nil
	}

	// Any other kind (e.g. git): deliver the packaged reference generically.
	return summary, nil, deliverToExternalTarget(octx, selected, spec, summary)
}

// packageIfNeeded packages the template for an apply/deploy. It is the only
// packaging entry point allowed to provision a missing packaging bucket
// (provision.backend.enabled: true); preview, validation and changeset creation
// go through prepareTemplateForAPI, which never creates infrastructure.
// Inline templates need no upload unless an S3 publish target was selected.
// A direct-deploy target needs TemplateURL just as much as an external delivery
// target: CreateChangeSet rejects TemplateBody over 51,200 bytes. Packaging first
// also prevents embedding an oversized body in a delivered artifact after that
// same template was already uploaded.
func packageIfNeeded(octx *opContext, provisionSection map[string]any, selected *target.SelectedTarget, spec *stackSpec, summary map[string]any) error {
	return packageTemplate(octx, &packagingRequest{
		ProvisionSection: provisionSection,
		Selected:         selected,
		Spec:             spec,
		Summary:          summary,
		MayProvision:     true,
	})
}

// prepareTemplateForAPI packages oversized templates before preview, validation, or changeset creation.
// It only selects a provision target when an upload is needed; an inline template
// can be inspected without a deployment destination or packaging configuration.
// Unlike apply, these read-style verbs never provision the packaging bucket: a
// missing bucket fails with a hint to run `backend create` or `apply`.
func prepareTemplateForAPI(octx *opContext, spec *stackSpec, summary map[string]any) error {
	if spec.TemplateURL != "" || !needsPackaging(spec.TemplateBody) {
		return nil
	}
	provisionSection, _ := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	flagTarget, _ := octx.Flags[targetKey].(string)
	selected, err := target.SelectTargetWithDefault(provisionSection, flagTarget, "default", cfg.CloudFormationComponentType)
	if err != nil {
		return err
	}
	return packageTemplate(octx, &packagingRequest{
		ProvisionSection: provisionSection,
		Selected:         selected,
		Spec:             spec,
		Summary:          summary,
		MayProvision:     false,
	})
}

// deployDirect executes the direct-deploy path. The changeset is created first
// so the user reviews what will change before anything is approved:
//
//	create changeset -> preview -> (no-op: stop) -> confirm -> execute
//
// A no-op deletes the FAILED "didn't contain changes" changeset and stops. A
// declined prompt, or any failure before execution starts, deletes the
// changeset (and the empty stub stack a first-time CREATE registered). The
// confirmation is skipped with --auto-approve, which deploy defaults to true.
func deployDirect(octx *opContext, client CloudFormationClient, spec *stackSpec) (*changeSetResult, error) {
	ctx := octx.Ctx
	result, err := createChangeSet(ctx, client, spec)
	if err != nil {
		discardChangeSet(ctx, client, spec.StackName, result)
		return nil, err
	}
	if result.NoOp {
		discardChangeSet(ctx, client, spec.StackName, result)
		announceNoChanges(spec.StackName)
		return result, nil
	}

	renderApplyPreview(spec.StackName, result)
	if err := confirmApply(octx, spec.StackName); err != nil {
		discardChangeSet(ctx, client, spec.StackName, result)
		return nil, err
	}

	if _, err := prepareStackPolicy(ctx, client, spec, result); err != nil {
		discardChangeSet(ctx, client, spec.StackName, result)
		return result, err
	}

	// Captured immediately before ExecuteChangeSet -- see preOperationEventBaseline
	// -- so streamStackEvents can tell a fast create/update's own events apart
	// from anything already present on the stack.
	baseline := preOperationEventBaseline(ctx, client, spec.StackName)

	if err := executeChangeSet(ctx, client, spec, result); err != nil {
		discardChangeSet(ctx, client, spec.StackName, result)
		return result, err
	}

	status, err := streamStackEvents(ctx, client, spec.StackName, baseline, OperationApply)
	if err != nil {
		return result, err
	}
	result.StackStatus = status
	if isFailedStackStatus(status) {
		return result, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationOperationFailed, spec.StackName, status)
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
		return nil, errUtils.Build(fmt.Errorf("%w: packaging target %q not found among aws/s3 targets", errUtils.ErrInvalidAwsCloudFormationSettings, packagingName)).
			WithExplanationf("The deploy target %q sets `packaging: %s`, but no `kind: aws/s3` provision target has that name.", selected.Name, packagingName).
			WithHintf("Set `packaging:` to one of the declared `kind: aws/s3` target names: %s.", strings.Join(sortedKeys(s3Targets), ", ")).
			Err()
	}

	return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
		WithExplanationf("Multiple `kind: aws/s3` provision targets are declared (%s); packaging is ambiguous.", strings.Join(sortedKeys(s3Targets), ", ")).
		WithHint("Declare the deploy target explicitly and name its packaging bucket, for example `provision: {default: deploy, targets: {deploy: {kind: aws/cloudformation, packaging: <s3-target-name>}}}`. The implicit default target cannot carry `packaging:`; a target named `default` replaces the implicit one.").
		Err()
}

// sortedKeys returns the map's keys in sorted order, for deterministic error text.
func sortedKeys(m map[string]map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
//
// Region is required (not just optional metadata): packageURL below needs it
// to build a CloudFormation-compatible https:// TemplateURL (AWS rejects a
// bare s3:// URI), and unlike the CloudFormation client's own region
// resolution (region.go's resolveRegion, which can defer to the active
// identity or the SDK default chain), there is no reliable way to recover the
// region the S3 upload actually used after the fact -- artifact.Backend's
// Upload returns only an error, no location/region back to the caller.
func s3ConfigFromTarget(name string, block map[string]any) (*targetS3Config, error) {
	s3cfg, err := s3ConfigFromTargetAllowEmptyRegion(name, block)
	if err != nil {
		return nil, err
	}
	if s3cfg.Region == "" {
		return nil, fmt.Errorf("%w: aws/s3 target %q is missing `region` (required to build a valid TemplateURL)", errUtils.ErrInvalidAwsCloudFormationSettings, name)
	}
	return s3cfg, nil
}

// s3ConfigFromTargetAllowEmptyRegion extracts bucket/prefix/region from a
// resolved `kind: aws/s3` target block without requiring `region` to be set.
// Used by the `atmos aws cloudformation backend` command group
// (ResolveS3BackendTarget/FindS3BackendTargets), where an empty target region
// is still resolvable via BuildSyntheticBackendConfig's fallback chain
// (resolveBackendRegion: settings.aws_cloudformation.region, then the active
// identity's AWS region). Rejecting an empty region here — the way
// s3ConfigFromTarget does for the packaging path, which has no such fallback
// available — would make that fallback chain unreachable. The final,
// fully-resolved region is validated later, once the fallback chain has had a
// chance to run (pkg/provisioner/backend/s3.go's extractS3Config errors on a
// still-empty region at that point).
func s3ConfigFromTargetAllowEmptyRegion(name string, block map[string]any) (*targetS3Config, error) {
	bucket, _ := block["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("%w: aws/s3 target %q is missing `bucket`", errUtils.ErrInvalidAwsCloudFormationSettings, name)
	}
	region, _ := block["region"].(string)
	prefix, _ := block["prefix"].(string)
	return &targetS3Config{Name: name, Bucket: bucket, Prefix: normalizeS3Prefix(prefix), Region: region}, nil
}

// deliverToExternalTarget publishes the packaged template to a non-direct-deploy
// provision target (e.g. `kind: git`) as a producer-agnostic ProvisionArtifact
// via the target registry — the same registry-based delivery Helm uses for its
// own non-cluster targets, just carrying a CloudFormation template instead of
// Kubernetes manifests.
func deliverToExternalTarget(octx *opContext, selected *target.SelectedTarget, spec *stackSpec, summary map[string]any) error {
	fileName := spec.StackName + ".yaml"
	content := templateContentForDelivery(spec)
	files := map[string][]byte{fileName: content}
	summary["template_bytes"] = len(content)

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

	if err := target.Deliver(deliverCtx, selected.Kind, &target.DeliverInput{
		AtmosConfig:  octx.AtmosConfig,
		TargetName:   selected.Name,
		TargetConfig: selected.Config,
		Artifact:     artifact,
		EnvProvider:  authManagerFor(octx.Info),
	}); err != nil {
		return err
	}
	reportDelivered(selected, fileName, summary)
	return nil
}

// templateContentForDelivery returns the bytes an external (e.g. git) target
// artifact should contain: when deliverApply already packaged the template
// (spec.TemplateURL set, because it needed packaging), a small pointer
// document referencing the uploaded location -- not the original oversized
// body, which packaging was specifically meant to avoid duplicating into
// every delivery destination. Otherwise, the raw template body is delivered
// as-is (the common case: no packaging was needed).
func templateContentForDelivery(spec *stackSpec) []byte {
	if spec.TemplateURL == "" {
		return []byte(spec.TemplateBody)
	}
	return []byte(fmt.Sprintf("# Packaged by atmos aws cloudformation -- the original template exceeded\n# CloudFormation's inline size limit and was uploaded here instead of being\n# embedded in this delivery.\nTemplateURL: %s\n", spec.TemplateURL))
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
