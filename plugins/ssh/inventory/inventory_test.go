package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/inventory"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

func TestListSkipsStoresThatDoNotExistOnThisPlatform(t *testing.T) {
	p := testfakes.Paths(t)
	store := keystore.NewFileStore(p.StoreDir("file"))
	for _, name := range []string{"b", "a"} {
		_, err := store.Create(context.Background(), name, nil)
		require.NoError(t, err)
	}

	keys, err := inventory.List(context.Background(), testfakes.FileOpener, p)

	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.Equal(t, []string{"a", "b"}, []string{keys[0].Name, keys[1].Name})
	assert.Equal(t, map[string]bool{
		authority.Fingerprint(keys[0].PublicKey): true, authority.Fingerprint(keys[1].PublicKey): true,
	}, inventory.Fingerprints(keys, nil))
}

func TestAStoreThatFailsMakesTheFingerprintSetUnknown(t *testing.T) {
	p := testfakes.Paths(t)
	_, err := keystore.NewFileStore(p.StoreDir("file")).Create(context.Background(), "a", nil)
	require.NoError(t, err)
	opener := func(kind, dir string) (keystore.Store, error) {
		if kind == keystore.KindSecureEnclave {
			return nil, errors.New("keychain is unavailable")
		}
		return testfakes.FileOpener(kind, dir)
	}

	keys, err := inventory.List(context.Background(), opener, p)

	require.ErrorContains(t, err, "keychain is unavailable")
	assert.Len(t, keys, 1, "the working store's keys are still returned")
	assert.Nil(t, inventory.Fingerprints(keys, err), "a partial set would make a live key look gone")
}
