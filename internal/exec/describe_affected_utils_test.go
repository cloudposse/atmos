package exec

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage"
	"github.com/stretchr/testify/assert"

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

// Helper function to create a mock repository with a valid HEAD
func createMockRepoWithHead(t *testing.T) *git.Repository {
	t.Helper()

	return &git.Repository{
		Storer: &mockStorer{
			head: plumbing.NewReferenceFromStrings(
				"refs/heads/master",
				"0123456789abcdef0123456789abcdef01234567",
			),
		},
	}
}

// Mock storer implementation
type mockStorer struct {
	head *plumbing.Reference
}

// Reference-related methods
func (s *mockStorer) Reference(name plumbing.ReferenceName) (*plumbing.Reference, error) {
	if name == plumbing.HEAD {
		return s.head, nil
	}
	return nil, errors.New("reference not found")
}

func (s *mockStorer) SetReference(*plumbing.Reference) error                              { return nil }
func (s *mockStorer) CheckAndSetReference(*plumbing.Reference, *plumbing.Reference) error { return nil }
func (s *mockStorer) RemoveReference(plumbing.ReferenceName) error                        { return nil }
func (s *mockStorer) CountLooseRefs() (int, error)                                        { return 0, nil }
func (s *mockStorer) PackRefs() error                                                     { return nil }
func (s *mockStorer) IterReferences() (storer.ReferenceIter, error)                       { return nil, nil }

// Object-related methods
func (s *mockStorer) NewEncodedObject() plumbing.EncodedObject {
	return nil
}

func (s *mockStorer) SetEncodedObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	return plumbing.ZeroHash, errors.New("not implemented")
}

func (s *mockStorer) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	return nil, errors.New("not implemented")
}

func (s *mockStorer) IterEncodedObjects(t plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	return nil, errors.New("not implemented")
}

// Added missing object-related methods
func (s *mockStorer) HasEncodedObject(h plumbing.Hash) error {
	return errors.New("not implemented")
}

func (s *mockStorer) EncodedObjectSize(h plumbing.Hash) (int64, error) {
	return 0, errors.New("not implemented")
}

// Index-related methods
func (s *mockStorer) SetIndex(object *index.Index) error {
	return nil
}

func (s *mockStorer) Index() (*index.Index, error) {
	return nil, errors.New("not implemented")
}

// Config-related methods
func (s *mockStorer) Config() (*config.Config, error) {
	return nil, errors.New("not implemented")
}

func (s *mockStorer) SetConfig(config *config.Config) error {
	return nil
}

// Shallow-related methods
func (s *mockStorer) Shallow() ([]plumbing.Hash, error) {
	return nil, nil
}

func (s *mockStorer) SetShallow([]plumbing.Hash) error {
	return nil
}

// Module-related methods
func (s *mockStorer) Module(name string) (storage.Storer, error) {
	return nil, errors.New("not implemented")
}

func (s *mockStorer) AddAlternate(remote string) error {
	return errors.New("not implemented")
}
