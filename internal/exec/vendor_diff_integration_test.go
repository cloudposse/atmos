package exec

//go:generate mockgen -source=vendor_git_interface.go -destination=mock_vendor_git_interface.go -package=exec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteVendorDiffWithGitOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		vendorYAML  string
		flags       *VendorDiffFlags
		mockSetup   func(*MockGitOperations)
		expectError bool
		expectedErr error
	}{
		{
			name: "successful diff with explicit from/to",
			vendorYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			flags: &VendorDiffFlags{
				Component: "vpc",
				From:      "v1.0.0",
				To:        "v1.2.0",
				Context:   3,
				NoColor:   true,
			},
			mockSetup: func(m *MockGitOperations) {
				m.EXPECT().
					GetDiffBetweenRefs(gomock.Any(), "https://github.com/cloudposse/terraform-aws-components", "v1.0.0", "v1.2.0", 3, true).
					Return([]byte("diff output"), nil)
			},
			expectError: false,
		},
		{
			name: "diff with automatic latest version detection",
			vendorYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			flags: &VendorDiffFlags{
				Component: "vpc",
				From:      "v1.0.0",
				To:        "", // Should auto-detect latest
				Context:   3,
				NoColor:   true,
			},
			mockSetup: func(m *MockGitOperations) {
				m.EXPECT().
					GetRemoteTags("https://github.com/cloudposse/terraform-aws-components").
					Return([]string{"v1.0.0", "v1.1.0", "v1.2.0", "v2.0.0"}, nil)
				m.EXPECT().
					GetDiffBetweenRefs(gomock.Any(), "https://github.com/cloudposse/terraform-aws-components", "v1.0.0", "v2.0.0", 3, true).
					Return([]byte("diff output"), nil)
			},
			expectError: false,
		},
		{
			name: "component not found error",
			vendorYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			flags: &VendorDiffFlags{
				Component: "nonexistent",
				From:      "v1.0.0",
				To:        "v1.2.0",
			},
			mockSetup:   func(m *MockGitOperations) {},
			expectError: true,
			expectedErr: errUtils.ErrComponentNotFound,
		},
		{
			name: "unsupported source type",
			vendorYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://registry/image
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			flags: &VendorDiffFlags{
				Component: "vpc",
				From:      "v1.0.0",
				To:        "v1.2.0",
			},
			mockSetup:   func(m *MockGitOperations) {},
			expectError: true,
			expectedErr: errUtils.ErrUnsupportedVendorSource,
		},
		{
			name: "no tags found error",
			vendorYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			flags: &VendorDiffFlags{
				Component: "vpc",
				From:      "v1.0.0",
				To:        "", // Should try to auto-detect but fail.
				Context:   3,
				NoColor:   true,
			},
			mockSetup: func(m *MockGitOperations) {
				m.EXPECT().
					GetRemoteTags("https://github.com/cloudposse/terraform-aws-components").
					Return([]string{}, nil) // Empty tags.
			},
			expectError: true,
			expectedErr: errUtils.ErrNoTagsFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temp directory and vendor.yaml.
			tempDir := t.TempDir()
			vendorFile := filepath.Join(tempDir, "vendor.yaml")
			err := os.WriteFile(vendorFile, []byte(tt.vendorYAML), 0o644)
			require.NoError(t, err)

			// Setup Atmos configuration.
			atmosConfig := &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					BasePath: vendorFile,
				},
			}

			// Setup mock.
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockGit := NewMockGitOperations(ctrl)
			tt.mockSetup(mockGit)

			// Execute.
			err = executeVendorDiffWithGitOps(atmosConfig, tt.flags, mockGit)

			// Assert.
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteVendorDiffWithGitOps_DefaultFromVersion(t *testing.T) {
	// This test verifies that when --from is not specified, it defaults to current version
	vendorYAML := `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.5.0
      targets:
        - components/terraform/vpc
`

	tempDir := t.TempDir()
	vendorFile := filepath.Join(tempDir, "vendor.yaml")
	err := os.WriteFile(vendorFile, []byte(vendorYAML), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		Vendor: schema.Vendor{
			BasePath: vendorFile,
		},
	}

	flags := &VendorDiffFlags{
		Component: "vpc",
		From:      "", // Should default to 1.5.0
		To:        "v2.0.0",
		Context:   3,
		NoColor:   true,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockGit := NewMockGitOperations(ctrl)

	// Expect the call with fromRef="1.5.0" (current version from vendor.yaml)
	mockGit.EXPECT().
		GetDiffBetweenRefs(gomock.Any(), "https://github.com/cloudposse/terraform-aws-components", "1.5.0", "v2.0.0", 3, true).
		Return([]byte("diff output"), nil)

	err = executeVendorDiffWithGitOps(atmosConfig, flags, mockGit)
	assert.NoError(t, err)
}
