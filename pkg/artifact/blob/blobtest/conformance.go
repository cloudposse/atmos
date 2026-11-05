package blobtest

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Keys and sizes the cases share.
const (
	keyObject  = "k"
	keyNested  = "dir/one.txt"
	keyAttrs   = "attrs.json"
	keyLock    = "lock"
	keyRef     = "ref"
	keyCounter = "counter"
	bodyX      = "x"

	sourceChunk    = 1024
	maxRetries     = 500
	counterBuffer  = 16
	listPageSize   = 10
	listPageGuard  = 100
	listKeyCount   = 53
	listMinPages   = 6
	delimiterPages = 20
	tailOffset     = 7
	clampedOffset  = 8
)

// Factory returns a fresh, empty store for one subtest. It must register cleanup with t.Cleanup
// and must not share state with stores returned by earlier calls.
type Factory func(t *testing.T) blob.Store

// RunStoreConformance runs the transport contract against stores built by factory.
func RunStoreConformance(t *testing.T, factory Factory) {
	defer perf.Track(nil, "blobtest.RunStoreConformance")()

	t.Helper()
	require.NotNil(t, factory, "a store factory is required")
	suite := &conformance{factory: factory}
	t.Run("PutGetRoundTrip", suite.putGetRoundTrip)
	t.Run("ZeroLengthObject", suite.zeroLength)
	t.Run("MissingObjects", suite.missing)
	t.Run("OverwriteChangesETag", suite.overwrite)
	t.Run("RejectsInvalidKeys", suite.invalidKeys)
	t.Run("AttributesRoundTrip", suite.attributes)
	t.Run("RangedRead", suite.ranged)
	t.Run("SizeMismatchLeavesNothing", suite.sizeMismatch)
	t.Run("FailedBodyLeavesNothing", suite.failedBody)
	t.Run("CanceledPutLeavesNothing", suite.canceled)
	t.Run("LargeStreamedBody", suite.largeStream)
	t.Run("ConditionalIfAbsent", suite.ifAbsent)
	t.Run("ConditionalIfMatch", suite.ifMatch)
	t.Run("ConditionalDelete", suite.conditionalDelete)
	t.Run("ConcurrentCreateIfAbsentHasOneWinner", suite.concurrentIfAbsent)
	t.Run("ConcurrentCompareAndSwapHasNoLostUpdate", suite.concurrentSwap)
	t.Run("ListPrefixAndOrder", suite.listPrefix)
	t.Run("ListPaginationVisitsEveryKeyOnce", suite.listPagination)
	t.Run("ListDelimiterRollsUpPrefixes", suite.listDelimiter)
}

// conformance holds the factory shared by the cases.
type conformance struct {
	factory Factory
}

// newStore builds a store for one subtest.
func (c *conformance) newStore(t *testing.T) blob.Store {
	t.Helper()
	store := c.factory(t)
	require.NotNil(t, store)
	return store
}

// put stores data unconditionally and fails the test on error.
func put(t *testing.T, store blob.Store, key string, data []byte) *blob.ObjectInfo {
	t.Helper()
	info, err := store.Put(context.Background(), key, bytes.NewReader(data), blob.PutOptions{Size: int64(len(data))})
	require.NoError(t, err)
	require.NotNil(t, info)
	return info
}

// read returns the full content of key.
func read(t *testing.T, store blob.Store, key string) []byte {
	t.Helper()
	object, err := store.Get(context.Background(), key, blob.GetOptions{})
	require.NoError(t, err)
	defer object.Body.Close()
	data, err := io.ReadAll(object.Body)
	require.NoError(t, err)
	return data
}

// requireAbsent asserts the key does not exist.
func requireAbsent(t *testing.T, store blob.Store, key string) {
	t.Helper()
	_, err := store.Stat(context.Background(), key, blob.StatOptions{})
	require.True(t, blob.IsNotFound(err), "expected %q to be absent, got %v", key, err)
}

// keysOf collects the keys of listed objects.
func keysOf(objects []blob.ObjectInfo) []string {
	keys := make([]string, 0, len(objects))
	for _, object := range objects {
		keys = append(keys, object.Key)
	}
	return keys
}
