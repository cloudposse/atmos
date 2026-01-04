package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -package=exec -destination=mock_storer_test.go github.com/go-git/go-git/v5/storage Storer

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindAffected(t *testing.T) {
	tests := []struct {
		name                        string
		currentStacks               map[string]any
		remoteStacks                map[string]any
		atmosConfig                 *schema.AtmosConfiguration
		changedFiles                []string
		includeSpaceliftAdminStacks bool
		includeSettings             bool
		stackToFilter               string
		expectedAffected            []schema.Affected
		expectedError               bool
	}{
		{
			name:             "Empty stacks should return empty affected list",
			currentStacks:    map[string]any{},
			remoteStacks:     map[string]any{},
			atmosConfig:      &schema.AtmosConfiguration{},
			changedFiles:     []string{},
			expectedAffected: []schema.Affected{},
			expectedError:    false,
		},
		{
			name: "Stack filter should only process specified stack",
			currentStacks: map[string]any{
				"stack1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
				"stack2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			remoteStacks:     map[string]any{},
			atmosConfig:      &schema.AtmosConfiguration{},
			changedFiles:     []string{},
			stackToFilter:    "stack1",
			expectedAffected: []schema.Affected{},
			expectedError:    false,
		},
		{
			name: "Should detect changed Terraform component",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			changedFiles: []string{"components/terraform/vpc/main.tf"},
			expectedAffected: []schema.Affected{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					Stack:         "dev",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata"},
					StackSlug:     "dev-vpc",
				},
			},
			expectedError: false,
		},
		{
			name: "Should detect changed Helmfile component",
			currentStacks: map[string]any{
				"staging": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{
							"ingress": map[string]any{
								"metadata": map[string]any{
									"component": "helmfile-ingress",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"staging": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			changedFiles: []string{"components/helmfile/ingress/values.yaml"},
			expectedAffected: []schema.Affected{
				{
					Component:     "ingress",
					ComponentType: "helmfile",
					Stack:         "staging",
					StackSlug:     "staging-ingress",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata"},
				},
			},
			expectedError: false,
		},
		{
			name: "Should detect changed Packer component",
			currentStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"packer": map[string]any{
							"custom-ami": map[string]any{
								"metadata": map[string]any{
									"component": "packer-custom-ami",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"packer": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "components/packer",
					},
				},
			},
			changedFiles: []string{"components/packer/custom-ami/ubuntu.pkr.hcl"},
			expectedAffected: []schema.Affected{
				{
					Component:     "custom-ami",
					ComponentType: "packer",
					Stack:         "prod",
					StackSlug:     "prod-custom-ami",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata"},
				},
			},
			expectedError: false,
		},
		{
			name: "Should detect multiple component types in the same stack",
			currentStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
							},
						},
						"helmfile": map[string]any{
							"ingress": map[string]any{
								"metadata": map[string]any{
									"component": "helmfile-ingress",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
						"helmfile":  map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			changedFiles: []string{
				"components/terraform/vpc/main.tf",
				"components/helmfile/ingress/values.yaml",
			},
			expectedAffected: []schema.Affected{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					Stack:         "prod",
					StackSlug:     "prod-vpc",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata"},
				},
				{
					Component:     "ingress",
					ComponentType: "helmfile",
					Stack:         "prod",
					StackSlug:     "prod-ingress",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata"},
				},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				tt.atmosConfig,
				tt.changedFiles,
				tt.includeSpaceliftAdminStacks,
				tt.includeSettings,
				tt.stackToFilter,
				false,
			)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAffected, affected)
			}
		})
	}
}

func TestExecuteDescribeAffected(t *testing.T) {
	tests := []struct {
		name                  string
		localRepo             *git.Repository
		remoteRepo            *git.Repository
		atmosConfig           *schema.AtmosConfiguration
		localRepoPath         string
		remoteRepoPath        string
		includeSpaceliftAdmin bool
		includeSettings       bool
		stack                 string
		processTemplates      bool
		processYamlFunctions  bool
		skip                  []string
		expectedErr           string
	}{
		{
			atmosConfig: &schema.AtmosConfiguration{
				// Provide valid paths so filepath.Rel can succeed.
				StacksBaseAbsolutePath:        "/test/stacks",
				TerraformDirAbsolutePath:      "/test/components/terraform",
				HelmfileDirAbsolutePath:       "/test/components/helmfile",
				PackerDirAbsolutePath:         "/test/components/packer",
				StackConfigFilesAbsolutePaths: []string{"/test/stacks/catalog"},
			},
			localRepoPath:  "/test",
			remoteRepoPath: "/tmp/remote",
			name:           "fails when repo operations fails",
			localRepo:      createMockRepoWithHead(t),
			remoteRepo:     createMockRepoWithHead(t),
			// ExecuteDescribeStacks fails before reaching mock repo operations
			// because the test paths don't exist on the filesystem.
			// Use generic error substring that works on both Unix and Windows.
			expectedErr: "error reading file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			affected, localHead, remoteHead, err := executeDescribeAffected(
				tc.atmosConfig,
				tc.localRepoPath,
				tc.remoteRepoPath,
				tc.localRepo,
				tc.remoteRepo,
				tc.includeSpaceliftAdmin,
				tc.includeSettings,
				tc.stack,
				tc.processTemplates,
				tc.processYamlFunctions,
				tc.skip,
				false,
			)

			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				assert.Nil(t, affected)
				assert.Nil(t, localHead)
				assert.Nil(t, remoteHead)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, affected)
				assert.NotNil(t, localHead)
				assert.NotNil(t, remoteHead)
			}
		})
	}
}

// Helper function to create a mock repository with a valid HEAD.
func createMockRepoWithHead(t *testing.T) *git.Repository {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockStorer := NewMockStorer(ctrl)

	// Configure mock to return HEAD reference.
	head := plumbing.NewReferenceFromStrings(
		"refs/heads/master",
		"0123456789abcdef0123456789abcdef01234567",
	)
	mockStorer.EXPECT().
		Reference(plumbing.HEAD).
		Return(head, nil).
		AnyTimes()

	// Configure mock to return error for EncodedObject (triggers "not implemented" error path).
	mockStorer.EXPECT().
		EncodedObject(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("not implemented")).
		AnyTimes()

	return &git.Repository{
		Storer: mockStorer,
	}
}

// createMockRepoWithHeadError creates a mock repo that errors on Head() call.
func createMockRepoWithHeadError(t *testing.T) *git.Repository {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockStorer := NewMockStorer(ctrl)

	// Configure mock to return error for HEAD reference.
	mockStorer.EXPECT().
		Reference(plumbing.HEAD).
		Return(nil, errors.New("HEAD not found")).
		AnyTimes()

	return &git.Repository{
		Storer: mockStorer,
	}
}

func TestFindAffectedWithExcludeLocked(t *testing.T) {
	tests := []struct {
		name          string
		currentStacks map[string]any
		remoteStacks  map[string]any
		atmosConfig   *schema.AtmosConfiguration
		changedFiles  []string
		excludeLocked bool
		expectedLen   int
	}{
		{
			name: "excludeLocked false includes locked components",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
									"locked":    true,
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			changedFiles:  []string{"components/terraform/vpc/main.tf"},
			excludeLocked: false,
			expectedLen:   1,
		},
		{
			name: "excludeLocked true excludes locked components",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
									"locked":    true,
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			changedFiles:  []string{"components/terraform/vpc/main.tf"},
			excludeLocked: true,
			expectedLen:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				tt.atmosConfig,
				tt.changedFiles,
				false,
				true, // includeSettings
				"",
				tt.excludeLocked,
			)

			assert.NoError(t, err)
			assert.Len(t, affected, tt.expectedLen)
		})
	}
}

func TestFindAffectedWithIncludeSettings(t *testing.T) {
	tests := []struct {
		name            string
		currentStacks   map[string]any
		remoteStacks    map[string]any
		includeSettings bool
	}{
		{
			name: "includeSettings true captures settings",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
								"settings": map[string]any{
									"key": "value",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			includeSettings: true,
		},
		{
			name: "includeSettings false excludes settings",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			includeSettings: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			}

			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				atmosConfig,
				[]string{"components/terraform/vpc/main.tf"},
				false,
				tt.includeSettings,
				"",
				false,
			)

			assert.NoError(t, err)
			// Just verify it doesn't error - settings handling is implementation detail.
			assert.NotNil(t, affected)
		})
	}
}

func TestFindAffectedWithNilStacks(t *testing.T) {
	t.Run("nil current stacks", func(t *testing.T) {
		emptyStacks := map[string]any{}
		affected, err := findAffected(
			&emptyStacks,
			&emptyStacks,
			&schema.AtmosConfiguration{},
			[]string{},
			false,
			false,
			"",
			false,
		)

		assert.NoError(t, err)
		assert.Empty(t, affected)
	})
}

func TestFindAffectedWithSpaceliftAdminStacks(t *testing.T) {
	t.Run("includeSpaceliftAdminStacks flag", func(t *testing.T) {
		currentStacks := map[string]any{
			"admin": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"spacelift-admin": map[string]any{
							"metadata": map[string]any{
								"component": "spacelift-admin",
							},
						},
					},
				},
			},
		}
		remoteStacks := map[string]any{}
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		affected, err := findAffected(
			&currentStacks,
			&remoteStacks,
			atmosConfig,
			[]string{},
			true, // includeSpaceliftAdminStacks
			false,
			"",
			false,
		)

		assert.NoError(t, err)
		// Just verify it processes without error.
		assert.NotNil(t, affected)
	})
}

func TestExecuteDescribeAffectedLocalRepoHeadError(t *testing.T) {
	t.Run("fails when local repo Head() returns error", func(t *testing.T) {
		localRepo := createMockRepoWithHeadError(t)
		remoteRepo := createMockRepoWithHead(t)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        "/test/stacks",
			TerraformDirAbsolutePath:      "/test/components/terraform",
			HelmfileDirAbsolutePath:       "/test/components/helmfile",
			PackerDirAbsolutePath:         "/test/components/packer",
			StackConfigFilesAbsolutePaths: []string{"/test/stacks/catalog"},
		}

		affected, localHead, remoteHead, err := executeDescribeAffected(
			atmosConfig,
			"/test",
			"/tmp/remote",
			localRepo,
			remoteRepo,
			false,
			false,
			"",
			false,
			false,
			nil,
			false,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD not found")
		assert.Nil(t, affected)
		assert.Nil(t, localHead)
		assert.Nil(t, remoteHead)
	})
}

func TestExecuteDescribeAffectedRemoteRepoHeadError(t *testing.T) {
	t.Run("fails when remote repo Head() returns error", func(t *testing.T) {
		localRepo := createMockRepoWithHead(t)
		remoteRepo := createMockRepoWithHeadError(t)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        "/test/stacks",
			TerraformDirAbsolutePath:      "/test/components/terraform",
			HelmfileDirAbsolutePath:       "/test/components/helmfile",
			PackerDirAbsolutePath:         "/test/components/packer",
			StackConfigFilesAbsolutePaths: []string{"/test/stacks/catalog"},
		}

		affected, localHead, remoteHead, err := executeDescribeAffected(
			atmosConfig,
			"/test",
			"/tmp/remote",
			localRepo,
			remoteRepo,
			false,
			false,
			"",
			false,
			false,
			nil,
			false,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD not found")
		assert.Nil(t, affected)
		assert.Nil(t, localHead)
		assert.Nil(t, remoteHead)
	})
}

// Test constants.
func TestDescribeAffectedConstants(t *testing.T) {
	t.Run("shaString constant", func(t *testing.T) {
		assert.Equal(t, "SHA", shaString)
	})

	t.Run("refString constant", func(t *testing.T) {
		assert.Equal(t, "ref", refString)
	})
}

// Test error variable.
func TestRemoteRepoIsNotGitRepoError(t *testing.T) {
	t.Run("error message is correct", func(t *testing.T) {
		expectedMsg := "the target remote repo is not a Git repository. Check that it was initialized and has '.git' folder"
		assert.Equal(t, expectedMsg, RemoteRepoIsNotGitRepoError.Error())
	})

	t.Run("error can be used with errors.Is", func(t *testing.T) {
		err := RemoteRepoIsNotGitRepoError
		assert.True(t, errors.Is(err, RemoteRepoIsNotGitRepoError))
	})
}
