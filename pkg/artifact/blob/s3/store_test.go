package s3

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/blobtest"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

func TestConformance(t *testing.T) {
	variants := map[string]struct {
		fake  []s3test.Option
		store []Option
	}{
		"single-request": {store: []Option{WithPartSize(DefaultPartSize)}},
		"multipart":      {store: []Option{WithPartSize(smallPart)}},
		"prefixed":       {store: []Option{WithPrefix("/team/artifacts/")}},
		"capped-pages":   {fake: []s3test.Option{s3test.WithPageCap(7)}},
	}
	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			blobtest.RunStoreConformance(t, func(t *testing.T) blob.Store {
				fake := newFake(t, variant.fake...)
				return newTestStore(t, fake, variant.store...)
			})
		})
	}
}

func TestNewValidates(t *testing.T) {
	_, err := New("")
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	s, err := New("bucket")
	require.NoError(t, err)
	assert.Equal(t, "s3", s.Name())
	assert.Equal(t, "bucket", s.Bucket())
	assert.Equal(t, DefaultPartSize, s.cfg.partSize)
	assert.Equal(t, DefaultConcurrency, s.cfg.concurrency)
}

func TestCapabilities(t *testing.T) {
	enabled, err := New("b")
	require.NoError(t, err)
	caps := enabled.Capabilities()
	assert.True(t, caps.ConditionalPut && caps.ConditionalDelete && caps.Multipart && caps.Tags && caps.Listing)

	disabled, err := New("b", WithConditionalMode(ConditionalDisabled))
	require.NoError(t, err)
	assert.False(t, disabled.Capabilities().ConditionalPut)
	assert.False(t, disabled.Capabilities().ConditionalDelete)
}

func TestKeyMappingWithPrefix(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake, WithPrefix("/p/q/"))
	putBytes(t, s, "a/b", []byte("x"), blob.PutOptions{})
	assert.Equal(t, []string{"p/q/a/b"}, fake.Keys())

	objects, _, err := blob.ListAll(context.Background(), s, blob.ListOptions{})
	require.NoError(t, err)
	require.Len(t, objects, 1)
	assert.Equal(t, "a/b", objects[0].Key, "listed keys are relative to the configured prefix")
}

func TestKeyValidationPortableAndOpaque(t *testing.T) {
	fake := newFake(t)
	strict := newTestStore(t, fake)
	_, err := strict.Put(context.Background(), "a/../b", nil, blob.PutOptions{})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)

	opaque := newTestStore(t, fake, WithOpaqueKeys())
	putBytes(t, opaque, "a//odd/../key", []byte("x"), blob.PutOptions{})
	assert.Contains(t, fake.Keys(), "a//odd/../key", "legacy key patterns are passed through untouched")
	_, err = opaque.Put(context.Background(), "", nil, blob.PutOptions{})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
}

func TestRangedGetSendsRangeHeader(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	putBytes(t, s, "k", []byte("0123456789"), blob.PutOptions{})
	fake.ResetRequests()

	object, err := s.Get(context.Background(), "k", blob.GetOptions{Offset: 2, Length: 3})
	require.NoError(t, err)
	require.NoError(t, object.Body.Close())
	assert.Equal(t, int64(10), object.Size)
	require.Len(t, fake.RequestsFor(s3test.OpGetObject), 1)
	assert.Equal(t, "bytes=2-4", fake.RequestsFor(s3test.OpGetObject)[0].Header.Get("Range"))

	_, err = s.Get(context.Background(), "k", blob.GetOptions{Offset: 50})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "416 maps to an invalid request")
}

func TestStatLoadsTagsOnlyWhenAsked(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	putBytes(t, s, "k", []byte("x"), blob.PutOptions{Tags: map[string]string{"a": "b"}})
	fake.ResetRequests()

	info, err := s.Stat(context.Background(), "k", blob.StatOptions{})
	require.NoError(t, err)
	assert.Nil(t, info.Tags)
	assert.Empty(t, fake.RequestsFor(s3test.OpGetObjectTagging))

	info, err = s.Stat(context.Background(), "k", blob.StatOptions{IncludeTags: true})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b"}, info.Tags)
	assert.Len(t, fake.RequestsFor(s3test.OpGetObjectTagging), 1)
}

func TestListPaginatesAcrossMoreThanAThousandKeys(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	const total = 2503
	for i := range total {
		fake.Seed(fmt.Sprintf("many/%05d", i), nil, nil)
	}
	fake.ResetRequests()

	objects, _, err := blob.ListAll(context.Background(), s, blob.ListOptions{Prefix: "many/"})
	require.NoError(t, err)
	require.Len(t, objects, total)
	assert.Equal(t, "many/00000", objects[0].Key)
	assert.Equal(t, fmt.Sprintf("many/%05d", total-1), objects[total-1].Key)
	assert.Len(t, fake.RequestsFor(s3test.OpListObjectsV2), 3, "1000 + 1000 + 503 entries")
	for _, object := range objects[:3] {
		assert.NotEmpty(t, object.ETag, "S3 listings carry ETags")
	}
}

func TestListDefaultsToServerPageSizeWhenNoLimit(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	fake.Seed("a", nil, nil)
	page, err := s.List(context.Background(), blob.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, page.NextCursor)
	assert.NotContains(t, fake.RequestsFor(s3test.OpListObjectsV2)[0].Query, "max-keys")
}
