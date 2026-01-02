package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// identityResolutionTestCase defines a test case for identity resolution functions.
type identityResolutionTestCase struct {
	name           string
	setupMock      func(*authTypes.MockAuthManager)
	identityFlag   string
	viperIdentity  string
	expectedResult string
	expectedError  error
}

// getIdentityResolutionTestCases returns common test cases for identity resolution.
func getIdentityResolutionTestCases() []identityResolutionTestCase {
	return []identityResolutionTestCase{
		{
			name: "uses default identity when no flag set",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("default-identity", nil)
			},
			identityFlag:   "",
			viperIdentity:  "",
			expectedResult: "default-identity",
			expectedError:  nil,
		},
		{
			name: "force select triggers interactive selection",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(true).Return("selected-identity", nil)
			},
			identityFlag:   IdentityFlagSelectValue,
			viperIdentity:  "",
			expectedResult: "selected-identity",
			expectedError:  nil,
		},
		{
			name: "no default identity returns error",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("", errUtils.ErrNoDefaultIdentity)
			},
			identityFlag:   "",
			viperIdentity:  "",
			expectedResult: "",
			expectedError:  errUtils.ErrNoDefaultIdentity,
		},
	}
}

// identityResolutionFunc is the signature for identity resolution functions.
type identityResolutionFunc func(*cobra.Command, *viper.Viper, authTypes.AuthManager) (string, error)

// runIdentityResolutionTests runs the common identity resolution tests for a given function.
func runIdentityResolutionTests(t *testing.T, fn identityResolutionFunc) {
	t.Helper()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tt := range getIdentityResolutionTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			mockAuthManager := authTypes.NewMockAuthManager(ctrl)
			tt.setupMock(mockAuthManager)

			// Create test command.
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String(IdentityFlagName, "", "identity")
			if tt.identityFlag != "" {
				_ = cmd.Flags().Set(IdentityFlagName, tt.identityFlag)
			}

			// Set up viper.
			v := viper.New()
			if tt.viperIdentity != "" {
				v.Set(IdentityFlagName, tt.viperIdentity)
			}

			result, err := fn(cmd, v, mockAuthManager)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestIdentityResolutionForShell(t *testing.T) {
	runIdentityResolutionTests(t, resolveIdentityNameForShell)
}

func TestIdentityResolutionForExec(t *testing.T) {
	runIdentityResolutionTests(t, resolveIdentityNameForExec)
}
