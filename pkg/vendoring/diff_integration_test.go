package vendoring

//go:generate mockgen -source=git_interface.go -destination=mock_git_interface.go -package=vendoring

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// setupTestIO initializes I/O context, data writer, and UI formatter for tests.
// This ensures tests that trigger data.Write() or ui.Infof() operations don't panic.
func setupTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to initialize I/O context: %v", err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

func TestExecuteVendorDiffWithGitOps(t *testing.T) {
	t.Parallel()
	// Initialize I/O before subtests to avoid race conditions.
	setupTestIO(t)

	tests := []struct {
		name        string
		vendorYAML  string
		flags       *diffFlags
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
			flags: &diffFlags{
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
			flags: &diffFlags{
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
			flags: &diffFlags{
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
			flags: &diffFlags{
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
			flags: &diffFlags{
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
	// Initialize I/O for data.Write() and ui.Infof() operations.
	setupTestIO(t)

	// This test verifies that when --from is not specified, it defaults to current version.
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

	flags := &diffFlags{
		Component: "vpc",
		From:      "", // Should default to 1.5.0
		To:        "v2.0.0",
		Context:   3,
		NoColor:   true,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockGit := NewMockGitOperations(ctrl)

	// Expect the call with fromRef="1.5.0" (current version from vendor.yaml).
	mockGit.EXPECT().
		GetDiffBetweenRefs(gomock.Any(), "https://github.com/cloudposse/terraform-aws-components", "1.5.0", "v2.0.0", 3, true).
		Return([]byte("diff output"), nil)

	err = executeVendorDiffWithGitOps(atmosConfig, flags, mockGit)
	assert.NoError(t, err)
}
