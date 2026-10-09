package cloudformation

import (
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// directTarget returns a `kind: aws/cloudformation` target block with extra keys.
func directTarget(extra map[string]any) map[string]any {
	block := map[string]any{"kind": cfg.CloudFormationComponentType}
	for key, value := range extra {
		block[key] = value
	}
	return block
}

// TestValidateDirectTargetKeys rejects keys a direct-deploy target silently ignores, in both real and dry runs.
func TestValidateDirectTargetKeys(t *testing.T) {
	for _, tc := range []struct {
		name     string
		targets  map[string]any
		wantErr  bool
		wantHint string
		wantKey  string
	}{
		{name: "kind only", targets: map[string]any{"direct": directTarget(nil)}},
		{name: "auth and packaging", targets: map[string]any{
			"direct":    directTarget(map[string]any{"auth": map[string]any{"identity": "dev"}, "packaging": "artifacts"}),
			"artifacts": s3TargetBlock(nil),
		}},
		{name: "s3 target keeps its region", targets: map[string]any{"artifacts": s3TargetBlock(nil)}},
		{
			name:     "region on a direct target",
			targets:  map[string]any{"direct": directTarget(map[string]any{"region": "us-west-2"})},
			wantErr:  true,
			wantHint: "settings.aws_cloudformation.region",
			wantKey:  "region",
		},
		{
			name:     "misspelled key",
			targets:  map[string]any{"direct": directTarget(map[string]any{"packging": "artifacts"})},
			wantErr:  true,
			wantHint: "`packaging`",
			wantKey:  "packging",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			section := dryRunSection("Resources: {}", tc.targets, nil)

			for mode, run := range map[string]func() error{
				"real run": func() error { return validateComponentConfig(section) },
				"dry run":  func() error { return runDryRun(t, section, nil, OperationApply) },
			} {
				err := run()
				if !tc.wantErr {
					require.NoError(t, err, mode)
					continue
				}
				require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationTargetKeyUnsupported, mode)
				target, _ := errUtils.GetContext(err, "target")
				assert.Equal(t, "direct", target, mode)
				assert.Contains(t, strings.Join(cockroachErrors.GetAllDetails(err), " "), tc.wantKey, mode)
				assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), tc.wantHint, mode)
			}
		})
	}
}

// TestDryRunValidatesSelectedTargetAuth makes a dry run reject the target auth blocks a real run would.
func TestDryRunValidatesSelectedTargetAuth(t *testing.T) {
	for _, tc := range []struct {
		name    string
		auth    any
		wantErr error
	}{
		{name: "valid identity", auth: map[string]any{"identity": "dev"}},
		{name: "valid default identity", auth: map[string]any{"identities": map[string]any{"dev": map[string]any{"default": true}}}},
		{name: "misspelled key", auth: map[string]any{"identitty": "dev"}, wantErr: errUtils.ErrProvisionTargetAuthUnknownKey},
		{name: "identity never selected", auth: map[string]any{"identities": map[string]any{"dev": map[string]any{}}}, wantErr: errUtils.ErrProvisionTargetAuthNoIdentity},
		{name: "scalar auth", auth: "dev", wantErr: errUtils.ErrProvisionTargetAuthInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			targets := map[string]any{"direct": directTarget(map[string]any{"auth": tc.auth})}
			section := dryRunSection("Resources: {}", targets, map[string]any{"default": "direct"})
			err := runDryRun(t, section, nil, OperationApply)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationTargetAuthFailed)
			assert.ErrorIs(t, err, tc.wantErr)
			name, _ := errUtils.GetContext(err, "target")
			assert.Equal(t, "direct", name)
			component, _ := errUtils.GetContext(err, "component")
			assert.Equal(t, "vpc", component)
		})
	}
}

// TestDryRunSkipsTargetAuthForTeardown keeps StackSet delete independent of delivery-target configuration.
func TestDryRunSkipsTargetAuthForTeardown(t *testing.T) {
	targets := map[string]any{"direct": directTarget(map[string]any{"auth": "invalid"})}
	section := dryRunSection("Resources: {}", targets, map[string]any{"default": "direct"})
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", ComponentSection: section}
	check := &dryRunCheck{AtmosConfig: &schema.AtmosConfiguration{}, Info: info, Section: section, Operation: OperationStackSetDelete}
	require.NoError(t, check.validateTargetAuth())
}
