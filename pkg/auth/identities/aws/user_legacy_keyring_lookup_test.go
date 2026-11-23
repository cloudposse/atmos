package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	atmosCredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

var errLegacyLookupFailed = errors.New("keyring backend unavailable")

// failingLegacyLookupStore is a credential store whose ambiguous legacy-entry lookup fails.
// The embedded CredentialStore is never called by the code under test.
type failingLegacyLookupStore struct {
	types.CredentialStore
	lookupCalls int
	adoptCalled bool
}

func (s *failingLegacyLookupStore) FindAmbiguousLegacyRealmEntry(string, string) (*atmosCredentials.AmbiguousLegacyRealmEntry, error) {
	s.lookupCalls++
	return nil, errLegacyLookupFailed
}

func (s *failingLegacyLookupStore) AdoptAmbiguousLegacyRealmEntry(string, string) error {
	s.adoptCalled = true
	return nil
}

// TestAdoptAmbiguousLegacyKeyringEntry_LookupFailureKeepsNotFoundHandling verifies that a failed
// lookup of the legacy entry is best effort: it neither prompts nor adopts anything, and returns
// (nil, nil) so the caller keeps its usual not-configured handling.
func TestAdoptAmbiguousLegacyKeyringEntry_LookupFailureKeepsNotFoundHandling(t *testing.T) {
	resetLegacyKeyringDeclines(t)
	ctrl := gomock.NewController(t)
	prompter := NewMocklegacyKeyringPrompter(ctrl) // No expectations: any prompt call fails the test.

	user := &userIdentity{name: legacyTestIdentity, realm: legacyTestRealm, legacyPrompter: prompter}
	store := &failingLegacyLookupStore{}

	creds, err := user.adoptAmbiguousLegacyKeyringEntry(context.Background(), store)

	require.NoError(t, err)
	assert.Nil(t, creds)
	assert.Equal(t, 1, store.lookupCalls, "the lookup must run exactly once, otherwise the test proves nothing")
	assert.False(t, store.adoptCalled, "a failed lookup must never adopt an entry")
}
