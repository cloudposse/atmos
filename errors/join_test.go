package errors

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinPreservingHints_NoErrors(t *testing.T) {
	assert.Nil(t, JoinPreservingHints())
	assert.Nil(t, JoinPreservingHints(nil, nil))
}

func TestJoinPreservingHints_SingleErrorNoHint(t *testing.T) {
	base := errors.New("boom")
	joined := JoinPreservingHints(base)

	require.Error(t, joined)
	assert.ErrorIs(t, joined, base)
	assert.Empty(t, errors.GetAllHints(joined))
}

func TestJoinPreservingHints_PreservesHintsFromEachError(t *testing.T) {
	first := Build(errors.New("first failed")).WithHint("fix the first thing").Err()
	second := Build(errors.New("second failed")).WithHint("fix the second thing").Err()

	joined := JoinPreservingHints(first, second)

	require.Error(t, joined)
	assert.ErrorIs(t, joined, first)
	assert.ErrorIs(t, joined, second)
	assert.ElementsMatch(t, []string{"fix the first thing", "fix the second thing"}, errors.GetAllHints(joined))
}

func TestJoinPreservingHints_DeduplicatesIdenticalHints(t *testing.T) {
	first := Build(errors.New("first failed")).WithHint("same hint").Err()
	second := Build(errors.New("second failed")).WithHint("same hint").Err()

	joined := JoinPreservingHints(first, second)

	assert.Equal(t, []string{"same hint"}, errors.GetAllHints(joined))
}

// A plain errors.Join loses hints attached to its sub-errors -- this is the
// exact failure this helper exists to fix. Asserting it here pins the
// regression down: if cockroachdb/errors ever starts traversing Unwrap()
// []error, this test (not just JoinPreservingHints' own tests) should be
// revisited.
func TestJoinPreservingHints_PlainJoinLosesHints(t *testing.T) {
	hinted := Build(errors.New("failed")).WithHint("a hint").Err()

	plainJoined := errors.Join(hinted)

	assert.Empty(t, errors.GetAllHints(plainJoined))
}
