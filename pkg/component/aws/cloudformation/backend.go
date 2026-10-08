package cloudformation

import (
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// provisionTimeout bounds one explicit `backend create`/`update` bucket reconcile.
const provisionTimeout = 5 * time.Minute

// backendTypeS3 mirrors pkg/provisioner/backend/s3.go's unexported
// backendTypeS3 constant. `atmos aws cloudformation backend` targets the same
// registered S3 backend type as `atmos terraform backend` — no new backend
// type is introduced for CFN's `kind: aws/s3` provision-target vocabulary.
const backendTypeS3 = "s3"

// backendMapKey is the raw-map key for a componentConfig's/synthetic
// backend-config map's "backend" section, at both provision.backend (the
// enabled flag) and top-level backend (bucket/region) — the same shape
// Terraform's own auto-provision hook reads.
const backendMapKey = "backend"

// ResolveS3BackendTarget finds the `kind: aws/s3` provision target that `atmos
// aws cloudformation backend` commands manage: the single declared target, or
// the one named by flagTarget when more than one is declared. Mirrors
// resolveStackSetTarget's (stackset.go) explicit-when-ambiguous resolution —
// there is no default among several aws/s3 targets, and reuses
// ErrInvalidAwsCloudFormationSettings rather than a bespoke sentinel, matching
// resolvePackagingTarget's (provision.go) identical no-target/ambiguous-target
// cases.
func ResolveS3BackendTarget(provisionSection map[string]any, flagTarget string) (*targetS3Config, error) {
	defer perf.Track(nil, "cloudformation.ResolveS3BackendTarget")()

	s3Targets := findS3Targets(provisionSection)

	if flagTarget != "" {
		block, ok := s3Targets[flagTarget]
		if !ok {
			return nil, fmt.Errorf("%w: %q is not a `kind: aws/s3` provision target", errUtils.ErrInvalidAwsCloudFormationSettings, flagTarget)
		}
		return s3ConfigFromTargetAllowEmptyRegion(flagTarget, block)
	}

	switch len(s3Targets) {
	case 0:
		return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
			WithExplanation("No `kind: aws/s3` provision target is declared.").
			WithHint("Add a `provision.targets.<name>: {kind: aws/s3, bucket: ...}` entry.").
			Err()
	case 1:
		for name, block := range s3Targets {
			return s3ConfigFromTargetAllowEmptyRegion(name, block)
		}
	}

	return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
		WithExplanation("Multiple `kind: aws/s3` provision targets are declared; selection is ambiguous.").
		WithHint("Pass --target <name> to select one.").
		Err()
}

// S3BackendTarget is the exported name of a resolved `kind: aws/s3` provision
// target, so callers outside this package can pass targets between helpers.
type S3BackendTarget = targetS3Config

// FindS3BackendTargets enumerates every `kind: aws/s3` provision target as a
// resolved targetS3Config, keyed by target name, for `backend list`. Targets
// missing a required `bucket` are silently skipped rather than failing the
// whole listing — s3ConfigFromTarget's error is only actionable when a single
// target was selected (ResolveS3BackendTarget), not while enumerating many.
func FindS3BackendTargets(provisionSection map[string]any) map[string]*targetS3Config {
	defer perf.Track(nil, "cloudformation.FindS3BackendTargets")()

	raw := findS3Targets(provisionSection)
	result := make(map[string]*targetS3Config, len(raw))
	for name, block := range raw {
		s3cfg, err := s3ConfigFromTargetAllowEmptyRegion(name, block)
		if err != nil {
			continue
		}
		result[name] = s3cfg
	}
	return result
}

// BuildSyntheticBackendConfig builds a componentConfig-shaped map matching
// what pkg/provisioner/backend.ProvisionBackend and
// pkg/provisioner.ProvisionWithParams expect from a Terraform component's own
// stack manifest (top-level `backend_type`/`backend`, plus
// `provision.backend.enabled`) — letting the CFN backend verbs reuse the
// already-registered S3 backend type (pkg/provisioner/backend/s3.go's
// backendTypeS3) instead of introducing a second one for CFN's `kind: aws/s3`
// provision-target vocabulary. This is the "narrow seam" adapter: it lives
// entirely in this package, and pkg/provisioner/provisioner.go and
// pkg/provisioner/backend/*.go are untouched.
//
// Region falls back through the target's own `region`, then
// `settings.aws_cloudformation.region` (resolveRegion — the same source
// apply/delete honor), then the active identity's resolved AWS region:
// pkg/provisioner/backend/s3.go's extractS3Config requires a non-empty region
// up front, before any AWS call is attempted.
//
// Prefix is intentionally omitted from the returned `backend` block:
// pkg/provisioner/backend/s3.go's extractS3Config never reads a `prefix` key
// (it is specific to the aws/s3 artifact store used for template packaging,
// packaging.go's uploadPackage), so including it here would be misleading.
func BuildSyntheticBackendConfig(s3cfg *targetS3Config, componentConfig map[string]any, authContext *schema.AuthContext) map[string]any {
	defer perf.Track(nil, "cloudformation.BuildSyntheticBackendConfig")()

	return map[string]any{
		"backend_type": backendTypeS3,
		backendMapKey: map[string]any{
			"bucket": s3cfg.Bucket,
			"region": resolveBackendRegion(s3cfg, componentConfig, authContext),
		},
		"provision": map[string]any{
			backendMapKey: map[string]any{"enabled": true},
		},
		// state_file_suffix/state_file_label override pkg/provisioner/backend/s3_delete.go's
		// default ".tfstate"/"Terraform state file(s)" deletion-warning wording, which is
		// meaningless for a CFN artifact bucket (it holds packaged templates, not Terraform
		// state). Empty suffix disables the sub-count/mention entirely.
		"state_file_suffix": "",
		"state_file_label":  "",
	}
}

// isBackendProvisionEnabled reports whether a raw component config declares
// `provision.backend.enabled: true` — the same top-level, sibling-to-targets
// shape Terraform's own auto-provision hook reads
// (pkg/provisioner/backend_hook.go's unexported isBackendProvisionEnabled;
// duplicated here rather than exported, since it's a two-key raw-map read).
func isBackendProvisionEnabled(componentConfig map[string]any) bool {
	provisionSection, ok := componentConfig["provision"].(map[string]any)
	if !ok {
		return false
	}
	backendSection, ok := provisionSection[backendMapKey].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := backendSection["enabled"].(bool)
	return ok && enabled
}

// autoProvisionArgs bundles autoProvisionBackendIfEnabled's inputs to stay
// under this repo's 5-argument function limit. The target S3Target MUST be
// the same one deliverApply already resolved via resolvePackagingTarget —
// never re-resolved independently, so auto-provisioning can never target a
// different bucket than the one apply is about to upload to.
type autoProvisionArgs struct {
	AtmosConfig     *schema.AtmosConfiguration
	S3Target        *targetS3Config
	ComponentConfig map[string]any
	AuthContext     *schema.AuthContext
	Component       string
	Stack           string
}

// autoProvisionBackendIfEnabled provisions the aws/s3 packaging target's
// bucket when the component declares `provision.backend.enabled: true` and
// the bucket doesn't already exist yet — the auto-provisioning behavior
// website/docs/migration/from-rain.mdx documents. Mirrors Terraform's own
// auto-provision hook (pkg/provisioner/backend_hook.go's autoProvisionBackend:
// existence-check-then-create, never reconciling an already-existing bucket
// on every single call) rather than calling provisioner.ProvisionWithParams
// unconditionally, which always reconciles and would cost every apply an AWS
// round-trip + spinner even once the bucket is already provisioned.
//
// Failure modes: enabled flag absent — silent no-op (unchanged behavior for
// every component not opting in). Existence-check failure — logged and
// deferred to uploadPackage's own error one step later (matches Terraform
// hook's own leniency: a transient check failure shouldn't block apply when
// the real answer will surface momentarily anyway). Creation failure — hard
// error; apply must not silently continue into uploadPackage against a
// bucket that may not exist.

func autoProvisionBackendIfEnabled(ctx context.Context, args autoProvisionArgs) error {
	defer perf.Track(args.AtmosConfig, "cloudformation.autoProvisionBackendIfEnabled")()

	if !isBackendProvisionEnabled(args.ComponentConfig) {
		return nil
	}

	synthetic := BuildSyntheticBackendConfig(args.S3Target, args.ComponentConfig, args.AuthContext)
	backendBlock, _ := synthetic[backendMapKey].(map[string]any)

	exists, err := backend.S3BackendExists(ctx, args.AtmosConfig, backendBlock, args.AuthContext)
	if err != nil {
		log.Debug("Backend existence check failed; deferring to template upload", "error", err, "bucket", args.S3Target.Bucket)
		return nil
	}
	if exists {
		return nil
	}

	describeFunc := func(string, string) (map[string]any, error) {
		return synthetic, nil
	}
	if err := provisioner.ProvisionWithParams(&provisioner.ProvisionParams{
		AtmosConfig:       args.AtmosConfig,
		ProvisionerType:   backendMapKey,
		Component:         args.Component,
		Stack:             args.Stack,
		DescribeComponent: describeFunc,
		AuthContext:       args.AuthContext,
		Context:           ctx,
	}); err != nil {
		return errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
			WithCause(err).
			WithExplanation(fmt.Sprintf("Failed to auto-provision the %q S3 backend (provision.backend.enabled: true)", args.S3Target.Bucket)).
			WithHint("Run `atmos aws cloudformation backend create` to see the full error, or set provision.backend.enabled: false and provision it manually.").
			Err()
	}
	return nil
}

// requireBackendExistsIfEnabled is autoProvisionBackendIfEnabled's read-only
// counterpart for operations that must never create infrastructure (validate,
// diff, changeset create). When the component opted in to
// `provision.backend.enabled: true` and the packaging bucket is missing, it
// fails with ErrAwsCloudFormationBackendMissing and a hint to create the bucket
// explicitly. An existence-check failure is logged and deferred to the upload's
// own error, matching autoProvisionBackendIfEnabled's leniency.
func requireBackendExistsIfEnabled(ctx context.Context, args autoProvisionArgs) error {
	defer perf.Track(args.AtmosConfig, "cloudformation.requireBackendExistsIfEnabled")()

	if !isBackendProvisionEnabled(args.ComponentConfig) {
		return nil
	}

	synthetic := BuildSyntheticBackendConfig(args.S3Target, args.ComponentConfig, args.AuthContext)
	backendBlock, _ := synthetic[backendMapKey].(map[string]any)

	exists, err := backend.S3BackendExists(ctx, args.AtmosConfig, backendBlock, args.AuthContext)
	if err != nil {
		log.Debug("Backend existence check failed; deferring to template upload", "error", err, "bucket", args.S3Target.Bucket)
		return nil
	}
	if exists {
		return nil
	}

	return errUtils.Build(errUtils.ErrAwsCloudFormationBackendMissing).
		WithExplanationf("The packaging bucket %q does not exist, and this command does not create it (only apply and deploy provision the bucket).", args.S3Target.Bucket).
		WithHintf("Run `atmos aws cloudformation backend create %s -s %s` to create the bucket, or run `atmos aws cloudformation apply %s -s %s`, which provisions it because provision.backend.enabled is true.", args.Component, args.Stack, args.Component, args.Stack).
		WithContext("bucket", args.S3Target.Bucket).
		Err()
}

// resolveBackendRegion implements BuildSyntheticBackendConfig's documented
// region fallback chain.
func resolveBackendRegion(s3cfg *targetS3Config, componentConfig map[string]any, authContext *schema.AuthContext) string {
	if s3cfg.Region != "" {
		return s3cfg.Region
	}
	if region := resolveRegion(componentConfig); region != "" {
		return region
	}
	if authContext != nil && authContext.AWS != nil {
		return authContext.AWS.Region
	}
	return ""
}

// S3BackendStatus is the result of checking whether a `kind: aws/s3` backend
// target's bucket exists, for `backend describe`/`backend list`.
//
// The yaml/json tags give the machine-readable output the snake_case keys the
// rest of this component type's JSON/YAML output uses.
//
// Error is set (and Exists is meaningless) when the target could not be
// inspected — for example because its identity failed to authenticate — so
// `backend list` can still render every other target.
type S3BackendStatus struct {
	Target *targetS3Config `json:"target" yaml:"target"`
	Region string          `json:"region" yaml:"region"`
	Exists bool            `json:"exists" yaml:"exists"`
	Error  string          `json:"error,omitempty" yaml:"error,omitempty"`
}

// NewS3BackendStatusError builds the status row for a target that could not be
// inspected, recording the cause so it can be rendered next to healthy targets.
func NewS3BackendStatusError(s3cfg *targetS3Config, cause error) *S3BackendStatus {
	defer perf.Track(nil, "cloudformation.NewS3BackendStatusError")()

	status := &S3BackendStatus{Target: s3cfg, Error: strings.Join(strings.Fields(cause.Error()), " ")}
	if s3cfg != nil {
		status.Region = s3cfg.Region
	}
	return status
}

// DescribeS3BackendTarget reports whether an aws/s3 backend target's bucket
// currently exists, calling pkg/provisioner/backend.S3BackendExists directly
// — a real (non-stub) implementation, unlike pkg/provisioner/provisioner.go's
// own DescribeBackend/ListBackends, which remain ErrNotImplemented stubs for
// Terraform today. CFN's describe/list need is narrower than Terraform's
// broader backend-type matrix (existence only), so it's implemented here
// rather than by fixing that pre-existing Terraform gap, which is out of
// scope for this change.
func DescribeS3BackendTarget(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	s3cfg *targetS3Config,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) (*S3BackendStatus, error) {
	defer perf.Track(atmosConfig, "cloudformation.DescribeS3BackendTarget")()

	synthetic := BuildSyntheticBackendConfig(s3cfg, componentConfig, authContext)
	backendBlock, _ := synthetic[backendMapKey].(map[string]any)

	exists, err := backend.S3BackendExists(ctx, atmosConfig, backendBlock, authContext)
	if err != nil {
		return nil, err
	}

	region, _ := backendBlock["region"].(string)
	return &S3BackendStatus{Target: s3cfg, Region: region, Exists: exists}, nil
}

// ProvisionS3BackendParams are the inputs to ProvisionS3BackendTarget. Target must come from
// ResolveS3BackendTarget.
type ProvisionS3BackendParams struct {
	AtmosConfig     *schema.AtmosConfiguration
	Target          *targetS3Config
	ComponentConfig map[string]any
	AuthContext     *schema.AuthContext
	Component       string
	Stack           string
}

// ProvisionS3BackendTarget creates the target's bucket, or reconciles an existing
// one to the secure defaults (versioning, encryption, public-access block, tags).
// It backs the explicit `backend create`/`backend update` verbs. Unlike
// provisioner.ProvisionWithParams (which prints its success line before the
// warnings it collected), it prints the "settings will be replaced" warnings
// first, so the final line reports the completed result.
func ProvisionS3BackendTarget(ctx context.Context, params *ProvisionS3BackendParams) error {
	defer perf.Track(params.AtmosConfig, "cloudformation.ProvisionS3BackendTarget")()

	synthetic := BuildSyntheticBackendConfig(params.Target, params.ComponentConfig, params.AuthContext)
	label := fmt.Sprintf("S3 backend `%s` for `%s` in stack `%s`", params.Target.Bucket, params.Component, params.Stack)
	ui.Info("Provisioning " + label)

	ctx, cancel := context.WithTimeout(ctx, provisionTimeout)
	defer cancel()

	result, err := backend.ProvisionBackend(ctx, params.AtmosConfig, synthetic, params.AuthContext)
	if err != nil {
		return err
	}
	if result != nil {
		for _, warning := range result.Warnings {
			ui.Warning(warning)
		}
	}
	ui.Success("Provisioned " + label)
	return nil
}

// S3BackendDryRunSummary resolves the `kind: aws/s3` target a backend verb would act on, from the
// component's static configuration alone (no AWS calls), and describes what the verb would do.
// The action argument is the verb phrase, for example "create" or "delete". It returns a wrapped
// ErrInvalidAwsCloudFormationSettings when no single target can be selected.
func S3BackendDryRunSummary(componentConfig map[string]any, flagTarget, action string) (string, error) {
	defer perf.Track(nil, "cloudformation.S3BackendDryRunSummary")()

	provisionSection, _ := componentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := ResolveS3BackendTarget(provisionSection, flagTarget)
	if err != nil {
		return "", err
	}
	region := resolveBackendRegion(s3cfg, componentConfig, nil)
	if region == "" {
		region = "the region of the active identity"
	}
	return fmt.Sprintf("would %s bucket %s in region %s", action, s3cfg.Bucket, region), nil
}
