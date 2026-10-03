package cloudformation

import (
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// directTargetKeys are the only keys a `kind: aws/cloudformation` target accepts.
// A direct-deploy target has no region of its own: the region comes from
// settings.aws_cloudformation.region, then the identity's region, then the SDK chain.
var directTargetKeys = map[string]bool{
	"kind":      true,
	"auth":      true,
	"packaging": true,
}

// validateDirectTargets rejects unsupported keys on `kind: aws/cloudformation`
// targets before any AWS call. Such a key would otherwise be accepted and
// silently ignored, for example a `region` that never moves the deployment.
func validateDirectTargets(config map[string]any) error {
	provision, _ := config[cfg.ProvisionSectionName].(map[string]any)
	targets, _ := provision["targets"].(map[string]any)
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		block, _ := targets[name].(map[string]any)
		if kind, _ := block["kind"].(string); kind != cfg.CloudFormationComponentType {
			continue
		}
		if err := validateDirectTargetKeys(config, name, block); err != nil {
			return err
		}
	}
	return nil
}

// validateDirectTargetKeys reports the unsupported keys of one direct-deploy target.
func validateDirectTargetKeys(config map[string]any, name string, block map[string]any) error {
	var unknown []string
	for key := range block {
		if !directTargetKeys[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	builder := errUtils.Build(errUtils.ErrAwsCloudFormationTargetKeyUnsupported).
		WithContext("target", name).
		WithExplanationf("The `kind: aws/cloudformation` target %q has unsupported key(s): %s. A direct-deploy target accepts only `kind`, `auth`, and `packaging`; other keys are ignored, so the deployment would not behave as written.", name, strings.Join(unknown, ", "))
	if stackName, _ := config[cfg.StackNameSectionName].(string); stackName != "" {
		builder = builder.WithContext("stack_name", stackName)
	}
	hint := "Remove the key, or move it to a supported place: `auth` selects the target's identity and `packaging` names the `kind: aws/s3` target that stores oversized templates."
	for _, key := range unknown {
		if key == "region" {
			hint = "A direct-deploy target has no region. Set `settings.aws_cloudformation.region` on the component; otherwise the identity's region and then the AWS SDK default chain apply. Only `kind: aws/s3` targets take a `region`."
			break
		}
	}
	return builder.WithHint(hint).Err()
}

// validateTargetAuth statically validates the selected delivery target's auth
// block, so a dry run rejects the same misconfiguration a real run would. A
// target that cannot be selected is reported by the other dry-run checks.
func (c *dryRunCheck) validateTargetAuth() error {
	octx := &opContext{AtmosConfig: c.AtmosConfig, Info: c.Info, Flags: c.Flags}
	// A selection error yields no block here and is reported by the other dry-run checks.
	name, block, _ := operationTargetConfig(octx, c.Operation)
	if block == nil {
		return nil
	}
	return validateTargetAuth(c.AtmosConfig, c.Info, name, block, cfg.NormalizeIdentityValue(c.Info.Identity))
}
