package blobtest

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// putWith stores data under a condition and returns the result.
func putWith(store blob.Store, key string, data []byte, cond blob.Condition) (*blob.ObjectInfo, error) {
	return store.Put(context.Background(), key, bytes.NewReader(data), blob.PutOptions{Size: int64(len(data)), Condition: cond})
}

func (c *conformance) ifAbsent(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().ConditionalPut {
		_, err := putWith(store, keyLock, []byte("a"), blob.IfAbsent())
		require.Error(t, err)
		assert.True(t, blob.IsConditionalUnsupported(err), "a store without conditional writes must say so: %v", err)
		requireAbsent(t, store, keyLock)
		return
	}
	first, err := putWith(store, keyLock, []byte("owner-a"), blob.IfAbsent())
	require.NoError(t, err)
	assert.NotEmpty(t, first.ETag)

	_, err = putWith(store, keyLock, []byte("owner-b"), blob.IfAbsent())
	require.Error(t, err)
	assert.True(t, blob.IsPreconditionFailed(err), "second create-if-absent must lose: %v", err)
	assert.False(t, blob.IsAmbiguous(err))
	assert.Equal(t, []byte("owner-a"), read(t, store, keyLock), "a lost race must not change the object")

	require.NoError(t, store.Delete(context.Background(), keyLock, blob.DeleteOptions{}))
	_, err = putWith(store, keyLock, []byte("owner-c"), blob.IfAbsent())
	require.NoError(t, err, "create-if-absent succeeds again after delete")
}

func (c *conformance) ifMatch(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().ConditionalPut {
		t.Skipf("store does not advertise conditional writes")
	}
	first := put(t, store, keyRef, []byte("v1"))

	second, err := putWith(store, keyRef, []byte("v2"), blob.IfMatch(first.ETag))
	require.NoError(t, err)
	assert.NotEqual(t, first.ETag, second.ETag)

	_, err = putWith(store, keyRef, []byte("v3"), blob.IfMatch(first.ETag))
	require.Error(t, err)
	assert.True(t, blob.IsPreconditionFailed(err), "stale token must lose: %v", err)
	assert.Equal(t, []byte("v2"), read(t, store, keyRef))

	_, err = putWith(store, "ghost", []byte(bodyX), blob.IfMatch(`"abc"`))
	require.Error(t, err)
	assert.True(t, blob.IsNotFound(err), "if-match on a missing object: %v", err)
	requireAbsent(t, store, "ghost")

	_, err = putWith(store, keyRef, []byte(bodyX), blob.Condition{Kind: blob.ConditionIfMatch})
	require.Error(t, err, "if-match without a token is invalid")
}

func (c *conformance) conditionalDelete(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().ConditionalDelete {
		put(t, store, keyObject, []byte(bodyX))
		err := store.Delete(context.Background(), keyObject, blob.DeleteOptions{IfMatch: `"abc"`})
		require.Error(t, err)
		assert.True(t, blob.IsConditionalUnsupported(err), "%v", err)
		assert.Equal(t, []byte(bodyX), read(t, store, keyObject))
		return
	}
	info := put(t, store, keyObject, []byte(bodyX))

	err := store.Delete(context.Background(), keyObject, blob.DeleteOptions{IfMatch: `"stale"`})
	require.Error(t, err)
	assert.True(t, blob.IsPreconditionFailed(err), "%v", err)
	assert.Equal(t, []byte(bodyX), read(t, store, keyObject), "a rejected delete must keep the object")

	require.NoError(t, store.Delete(context.Background(), keyObject, blob.DeleteOptions{IfMatch: info.ETag}))
	requireAbsent(t, store, keyObject)

	err = store.Delete(context.Background(), keyObject, blob.DeleteOptions{IfMatch: info.ETag})
	assert.True(t, blob.IsNotFound(err), "conditional delete of a missing object: %v", err)
}

func (c *conformance) concurrentIfAbsent(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().ConditionalPut {
		t.Skipf("store does not advertise conditional writes")
	}
	const writers = 12
	var (
		wg      sync.WaitGroup
		winners atomic.Int32
		losers  atomic.Int32
		start   = make(chan struct{})
	)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := putWith(store, "claim", []byte("owner-"+strconv.Itoa(i)), blob.IfAbsent())
			switch {
			case err == nil:
				winners.Add(1)
			case blob.IsPreconditionFailed(err):
				losers.Add(1)
			default:
				t.Errorf("writer %d: unexpected error %v", i, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, int32(1), winners.Load(), "exactly one creator wins")
	assert.Equal(t, int32(writers-1), losers.Load())
}

func (c *conformance) concurrentSwap(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().ConditionalPut {
		t.Skipf("store does not advertise conditional writes")
	}
	put(t, store, keyCounter, []byte("0"))
	const workers, increments = 6, 5
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range increments {
				incrementWithRetry(t, store)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, strconv.Itoa(workers*increments), string(read(t, store, keyCounter)), "every successful swap must be reflected")
}

// incrementWithRetry performs a read-modify-write loop guarded by If-Match.
func incrementWithRetry(t *testing.T, store blob.Store) {
	t.Helper()
	for range maxRetries {
		stat, err := store.Stat(context.Background(), keyCounter, blob.StatOptions{})
		if err != nil {
			t.Errorf("stat: %v", err)
			return
		}
		object, err := store.Get(context.Background(), keyCounter, blob.GetOptions{})
		if err != nil {
			t.Errorf("get: %v", err)
			return
		}
		raw := make([]byte, counterBuffer)
		n, _ := object.Body.Read(raw)
		_ = object.Body.Close()
		current, err := strconv.Atoi(string(raw[:n]))
		if err != nil {
			continue
		}
		next := []byte(fmt.Sprint(current + 1))
		_, err = putWith(store, keyCounter, next, blob.IfMatch(stat.ETag))
		if err == nil {
			return
		}
		if !blob.IsPreconditionFailed(err) {
			t.Errorf("swap: %v", err)
			return
		}
	}
	t.Errorf("swap did not converge")
}
