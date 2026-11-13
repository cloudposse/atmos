package blob

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestErrorKindsMatchSentinelsAndCauses(t *testing.T) {
	cause := errors.New("sdk detail")
	cases := []struct {
		kind     Kind
		sentinel error
		check    func(error) bool
	}{
		{KindNotFound, errUtils.ErrBlobNotFound, IsNotFound},
		{KindPrecondition, errUtils.ErrBlobPreconditionFailed, IsPreconditionFailed},
		{KindUnsupported, errUtils.ErrBlobConditionalUnsupported, IsConditionalUnsupported},
		{KindAmbiguous, errUtils.ErrBlobOutcomeUnknown, IsAmbiguous},
		{KindInvalid, errUtils.ErrBlobInvalid, func(err error) bool { return errors.Is(err, errUtils.ErrBlobInvalid) }},
		{KindIdentity, errUtils.ErrBlobIdentityUnresolved, func(err error) bool { return errors.Is(err, errUtils.ErrBlobIdentityUnresolved) }},
	}
	for _, tc := range cases {
		err := fmt.Errorf("wrapped: %w", NewError(tc.kind, "put", "k", cause))
		assert.ErrorIs(t, err, tc.sentinel)
		assert.ErrorIs(t, err, cause, "the cause stays visible")
		assert.True(t, tc.check(err))
		assert.Contains(t, err.Error(), `blob put "k"`)
		assert.Contains(t, err.Error(), "sdk detail")
	}
	assert.False(t, IsNotFound(NewError(KindPrecondition, "put", "k", nil)))
}

func TestErrorMessagesDoNotDuplicateTheSentinel(t *testing.T) {
	err := NewError(KindNotFound, "get", "k", fmt.Errorf("%w: gone", errUtils.ErrBlobNotFound))
	assert.Equal(t, 1, strings.Count(err.Error(), errUtils.ErrBlobNotFound.Error()))
	assert.Equal(t, `blob get "k": blob object not found`, NewError(KindNotFound, "get", "k", nil).Error())
	assert.Equal(t, `blob list: failed`, NewError(KindOther, "list", "", nil).Error())
	assert.Equal(t, `blob list: boom`, NewError(KindOther, "list", "", errors.New("boom")).Error())
}

func TestOnlyAmbiguousErrorsAreTemporary(t *testing.T) {
	for kind := KindOther; kind <= KindIdentity; kind++ {
		assert.Equal(t, kind == KindAmbiguous, NewError(kind, "put", "k", nil).Temporary(), "kind %d", kind)
	}
	var temporary interface{ Temporary() bool }
	require.True(t, errors.As(NewError(KindAmbiguous, "put", "k", nil), &temporary))
}

func TestValidateKey(t *testing.T) {
	valid := []string{"a", "a/b", "dir/file.tar", "x/y.z/w", strings.Repeat("a", MaxKeyBytes)}
	for _, key := range valid {
		assert.NoError(t, ValidateKey(key), key)
	}
	invalid := []string{"", "/a", "a/", "a//b", "a/./b", "a/../b", "..", ".", "a\\b", "a\x00b", "\xff", strings.Repeat("a", MaxKeyBytes+1)}
	for _, key := range invalid {
		err := ValidateKey(key)
		require.Error(t, err, key)
		assert.ErrorIs(t, err, errUtils.ErrBlobInvalid, key)
	}
}

func TestValidateOpaqueKeyAllowsLegacyPatterns(t *testing.T) {
	for _, key := range []string{"a//b", "/leading", "trailing/", "a/../b", "a\\b"} {
		assert.NoError(t, ValidateOpaqueKey(key), key)
	}
	for _, key := range []string{"", "a\x00b", strings.Repeat("a", MaxKeyBytes+1)} {
		assert.Error(t, ValidateOpaqueKey(key), key)
	}
}

func TestMergeTagsNeverAliasesInputs(t *testing.T) {
	defaults := map[string]string{"a": "1", "b": "2"}
	overrides := map[string]string{"b": "3", "c": "4"}
	merged := MergeTags(defaults, overrides)
	assert.Equal(t, map[string]string{"a": "1", "b": "3", "c": "4"}, merged)

	merged["a"] = "mutated"
	assert.Equal(t, "1", defaults["a"], "result to source isolation")
	defaults["z"] = "late"
	assert.NotContains(t, merged, "z", "source to result isolation")
	assert.Equal(t, "2", map[string]string{"a": "1", "b": "2"}["b"])
	assert.Nil(t, MergeTags(nil, nil))
	assert.Nil(t, CloneMap(nil))
	clone := CloneMap(overrides)
	clone["c"] = "x"
	assert.Equal(t, "4", overrides["c"])
}

func TestValidateTags(t *testing.T) {
	assert.NoError(t, ValidateTags(nil))
	assert.NoError(t, ValidateTags(map[string]string{"k": strings.Repeat("v", MaxTagValueChars)}))

	many := map[string]string{}
	for i := range MaxTags + 1 {
		many[fmt.Sprint("k", i)] = "v"
	}
	for name, tags := range map[string]map[string]string{
		"too many":  many,
		"empty key": {"": "v"},
		"long key":  {strings.Repeat("k", MaxTagKeyChars+1): "v"},
		"long val":  {"k": strings.Repeat("v", MaxTagValueChars+1)},
	} {
		assert.ErrorIs(t, ValidateTags(tags), errUtils.ErrBlobInvalid, name)
	}
}

func TestConditionHelpers(t *testing.T) {
	assert.False(t, Condition{}.IsConditional())
	assert.True(t, IfAbsent().IsConditional())
	assert.Equal(t, Condition{Kind: ConditionIfMatch, ETag: `"x"`}, IfMatch(`"x"`))
	assert.False(t, GetOptions{}.IsRange())
	assert.True(t, GetOptions{Offset: 1}.IsRange())
	assert.True(t, GetOptions{Length: 1}.IsRange())
}

func TestCapabilityChecksFailClearly(t *testing.T) {
	none := Capabilities{}
	assert.NoError(t, none.CheckPut("s", "k", Condition{}))
	err := none.CheckPut("s", "k", IfAbsent())
	assert.True(t, IsConditionalUnsupported(err))
	assert.Contains(t, err.Error(), "s store cannot enforce conditional writes")

	assert.NoError(t, none.CheckDelete("s", "k", DeleteOptions{}))
	assert.True(t, IsConditionalUnsupported(none.CheckDelete("s", "k", DeleteOptions{IfMatch: "x"})))

	full := Capabilities{ConditionalPut: true, ConditionalDelete: true}
	assert.NoError(t, full.CheckPut("s", "k", IfMatch("x")))
	assert.NoError(t, full.CheckDelete("s", "k", DeleteOptions{IfMatch: "x"}))
}

func TestListAllFollowsCursorsAndMergesPrefixes(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	pages := []ListPage{
		{Objects: []ObjectInfo{{Key: "a"}, {Key: "b"}}, CommonPrefixes: []string{"p/"}, NextCursor: "c1"},
		{Objects: []ObjectInfo{{Key: "c"}}, NextCursor: "c2"},
		{Objects: []ObjectInfo{{Key: "d"}}, CommonPrefixes: []string{"q/"}},
	}
	var calls []ListOptions
	store.EXPECT().List(gomock.Any(), gomock.Any()).Times(len(pages)).
		DoAndReturn(func(_ context.Context, opts ListOptions) (*ListPage, error) {
			calls = append(calls, opts)
			page := pages[len(calls)-1]
			return &page, nil
		})

	objects, prefixes, err := ListAll(context.Background(), store, ListOptions{Prefix: "x/", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c", "d"}, []string{objects[0].Key, objects[1].Key, objects[2].Key, objects[3].Key})
	assert.Equal(t, []string{"p/", "q/"}, prefixes)
	require.Len(t, calls, 3)
	assert.Equal(t, []string{"", "c1", "c2"}, []string{calls[0].Cursor, calls[1].Cursor, calls[2].Cursor})
	for _, call := range calls {
		assert.Equal(t, "x/", call.Prefix)
		assert.Equal(t, 2, call.Limit)
	}
}

func TestListAllStopsOnError(t *testing.T) {
	boom := errors.New("boom")
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	store.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, boom)
	_, _, err := ListAll(context.Background(), store, ListOptions{})
	require.ErrorIs(t, err, boom)
}

func TestValidateTagsRejectsInvalidUTF8(t *testing.T) {
	for _, tags := range []map[string]string{{"\xff": "value"}, {"key": "\xff"}} {
		require.ErrorIs(t, ValidateTags(tags), errUtils.ErrBlobInvalid)
	}
}

func TestValidateTagsCountsUTF16CodeUnits(t *testing.T) {
	const emoji = "\U0001F600" // One rune, two UTF-16 code units.
	keyAtLimit := strings.Repeat(emoji, MaxTagKeyChars/2)
	valueAtLimit := strings.Repeat(emoji, MaxTagValueChars/2)

	require.NoError(t, ValidateTags(map[string]string{keyAtLimit: valueAtLimit}), "exactly at both limits")
	for name, tags := range map[string]map[string]string{
		"key one unit over":    {keyAtLimit + "a": "v"},
		"key one rune over":    {keyAtLimit + emoji: "v"},
		"value one unit over":  {"k": valueAtLimit + "a"},
		"value one rune over":  {"k": valueAtLimit + emoji},
		"rune count under cap": {strings.Repeat(emoji, MaxTagKeyChars/2+1): "v"},
	} {
		require.ErrorIs(t, ValidateTags(tags), errUtils.ErrBlobInvalid, name)
	}
}
