package exec

//go:generate go run go.uber.org/mock/mockgen@latest -package=exec -destination=mock_storer_test.go github.com/go-git/go-git/v5/storage Storer

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
			atmosConfig: &schema.AtmosConfiguration{},
			name:        "fails when repo operations fails",
			localRepo:   createMockRepoWithHead(t),
			remoteRepo:  createMockRepoWithHead(t),
			expectedErr: "not implemented",
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
