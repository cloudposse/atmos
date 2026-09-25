package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_terraform_output_backend_test.go -package=exec github.com/cloudposse/atmos/pkg/terraform/output BackendGenerator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

func TestDeferredOutputDoesNotReuseCallerAuthContext(t *testing.T) {
	callerContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "caller-account"}}
	targetContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "target-account"}}
	for _, test := range []struct {
		name      string
		manager   bool
		stackInfo *schema.ConfigAndStacksInfo
		want      *schema.AuthContext
	}{
		{name: "no target identity"},
		{name: "manager without stack info", manager: true},
		{name: "manager without credentials", manager: true, stackInfo: &schema.ConfigAndStacksInfo{}},
		{name: "resolved target credentials", manager: true, stackInfo: &schema.ConfigAndStacksInfo{AuthContext: targetContext}, want: targetContext},
	} {
		for _, maskOnly := range []bool{false, true} {
			name := test.name
			if maskOnly {
				name += " masked secrets"
			}
			t.Run(name, func(t *testing.T) {
				ac, factory, _ := setupDeferredCacheTarget(t)
				var manager auth.AuthManager
				if test.manager {
					manager = newStackInfoAuthManager(t, test.stackInfo)
				}
				factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(manager, nil).Times(1)

				// Observe credentials at the output executor boundary and stop before any subprocess runs.
				backend := NewMockBackendGenerator(gomock.NewController(t))
				backend.EXPECT().GenerateBackendIfNeeded(gomock.Any(), "target", "dev", test.want).
					Return(errUtils.ErrTerraformOutputFailed)
				executor := tfoutput.GetDefaultExecutor()
				tfoutput.SetDefaultExecutor(tfoutput.NewExecutor(&componentDescriberAdapter{}, tfoutput.WithBackendGenerator(backend)))
				t.Cleanup(func() { tfoutput.SetDefaultExecutor(executor) })

				getter := &defaultOutputGetter{}
				_, _, err := getter.GetOutput(ac, "dev", "target", "id", true, callerContext, nil,
					TerraformLookupOptions{SecretsMaskOnly: maskOnly})
				require.ErrorIs(t, err, errUtils.ErrTerraformOutputFailed)
			})
		}
	}
}
