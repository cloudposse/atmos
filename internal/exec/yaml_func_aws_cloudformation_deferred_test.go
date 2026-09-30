package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCloudFormationOutputDeferredAuth(t *testing.T) {
	caller := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "caller-account"}}
	target := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "target-account"}}
	for _, tt := range []struct {
		name    string
		manager bool
		info    *schema.ConfigAndStacksInfo
		want    *schema.AWSAuthContext
	}{
		{name: "no target identity"},
		{name: "manager without stack info", manager: true},
		{name: "manager without credentials", manager: true, info: &schema.ConfigAndStacksInfo{}},
		{name: "target credentials", manager: true, info: &schema.ConfigAndStacksInfo{AuthContext: target}, want: target.AWS},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ac := setupAwsCloudFormationOutputFixture(t)
			ctrl := gomock.NewController(t)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			setDeferredAuthFactory(&ac, factory)
			var manager auth.AuthManager
			if tt.manager {
				manager = newStackInfoAuthManager(t, tt.info)
			}
			factory.EXPECT().Create(&ac, gomock.Any(), "test").Return(manager, nil)
			getter := NewMockCloudFormationOutputsGetter(ctrl)
			getter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", tt.want).Return(map[string]any{"VpcId": "target-vpc"}, nil)
			stubCloudFormationOutputsGetter(t, getter)
			value, err := processTagAwsCloudFormationOutputWithContext(&ac, "!aws.cloudformation.output vpc test VpcId", "test", nil,
				&schema.ConfigAndStacksInfo{AuthContext: caller})
			require.NoError(t, err)
			require.Equal(t, "target-vpc", value)
		})
	}
}

func TestCloudFormationOutputDeferredAuthFailure(t *testing.T) {
	ac := setupAwsCloudFormationOutputFixture(t)
	ctrl := gomock.NewController(t)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	setDeferredAuthFactory(&ac, factory)
	factory.EXPECT().Create(&ac, gomock.Any(), "test").Return(nil, errUtils.ErrInvalidAuthConfig)
	stubCloudFormationOutputsGetter(t, NewMockCloudFormationOutputsGetter(ctrl))
	_, err := processTagAwsCloudFormationOutputWithContext(&ac, "!aws.cloudformation.output vpc test VpcId", "test", nil,
		&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "caller-account"}}})
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestCloudFormationOutputAuthDisabled(t *testing.T) {
	for _, mode := range []string{"parent", "deferred parent", "deferred manager"} {
		t.Run(mode, func(t *testing.T) {
			ac := setupAwsCloudFormationOutputFixture(t)
			ctrl := gomock.NewController(t)
			info := &schema.ConfigAndStacksInfo{
				AuthContext:  &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "stale-caller"}},
				AuthDisabled: mode != "deferred manager",
			}
			if mode != "parent" {
				ac.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{
					Disabled: mode == "deferred manager", Factory: authdeferred.NewMockAuthFactory(ctrl),
				})
			}
			getter := NewMockCloudFormationOutputsGetter(ctrl)
			getter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", nil).Return(map[string]any{"VpcId": "ambient-vpc"}, nil)
			stubCloudFormationOutputsGetter(t, getter)
			value, err := processTagAwsCloudFormationOutputWithContext(&ac, "!aws.cloudformation.output vpc test VpcId", "test", nil, info)
			require.NoError(t, err)
			require.Equal(t, "ambient-vpc", value)
		})
	}
}

func TestCloudFormationDeferredAuthInheritsCallerConfiguration(t *testing.T) {
	for _, useTemplate := range []bool{false, true} {
		t.Run(fmt.Sprint(useTemplate), func(t *testing.T) {
			ac := setupAwsCloudFormationOutputFixture(t)
			clearComponentFuncSyncMap(t)
			ctrl := gomock.NewController(t)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			setDeferredAuthFactory(&ac, factory)
			getter := NewMockCloudFormationOutputsGetter(ctrl)
			stubCloudFormationOutputsGetter(t, getter)
			for _, profile := range []string{"first", "second"} {
				authContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: profile}}
				manager := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: authContext})
				factory.EXPECT().Create(&ac, gomock.Any(), "test").DoAndReturn(
					func(_ *schema.AtmosConfiguration, config *schema.AuthConfig, _ string) (auth.AuthManager, error) {
						require.Equal(t, profile, config.Identities["local"].Credentials["profile"])
						return manager, nil
					},
				)
				calls := 2
				if useTemplate {
					calls = 1 // atmos.Component caches outputs per effective target identity.
				}
				getter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", authContext.AWS).
					Return(map[string]any{"VpcId": profile}, nil).Times(calls)
				for range 2 {
					info := deferredCacheParent(profile)
					info.AuthManager = newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: authContext})
					var value any
					var err error
					if useTemplate {
						value, err = componentFunc(&ac, info, "vpc", "test")
						require.NoError(t, err)
						value = value.(map[string]any)["outputs"].(map[string]any)["VpcId"]
					} else {
						value, err = processTagAwsCloudFormationOutputWithContext(&ac, "!aws.cloudformation.output vpc test VpcId", "test", nil, info)
					}
					require.NoError(t, err)
					require.Equal(t, profile, value)
				}
			}
		})
	}
}

func TestCloudFormationOutputDeferredAuthReachesNestedYAML(t *testing.T) {
	// Copy the fixture so the nested credential consumer is local to this test.
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "aws-cloudformation-outputs"))))
	stackPath := filepath.Join(dir, "stacks", "test.yaml")
	contents, err := os.ReadFile(stackPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stackPath, []byte(strings.ReplaceAll(string(contents), "region: us-east-1", "region: !aws.region")), 0o600))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	setDeferredAuthFactory(&ac, factory)
	target := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "target-account"}}
	manager := newStackInfoAuthManager(t, &schema.ConfigAndStacksInfo{AuthContext: target})
	factory.EXPECT().Create(&ac, gomock.Any(), "test").Return(manager, nil)
	identity := awsIdentity.NewMockGetter(ctrl)
	identity.EXPECT().GetCallerIdentity(gomock.Any(), gomock.Any(), target.AWS).
		Return(&awsIdentity.CallerIdentity{Region: "eu-west-1"}, nil)
	awsIdentity.ClearIdentityCache()
	t.Cleanup(awsIdentity.ClearIdentityCache)
	t.Cleanup(awsIdentity.SetGetter(identity))
	getter := NewMockCloudFormationOutputsGetter(ctrl)
	getter.EXPECT().GetOutputs(gomock.Any(), "eu-west-1", "test-vpc", target.AWS).Return(map[string]any{"VpcId": "target-vpc"}, nil)
	stubCloudFormationOutputsGetter(t, getter)
	value, err := processTagAwsCloudFormationOutputWithContext(&ac, "!aws.cloudformation.output vpc test VpcId", "test", nil, nil)
	require.NoError(t, err)
	require.Equal(t, "target-vpc", value)
}
