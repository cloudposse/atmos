package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindCommandByName_UniqueNestedMatch(t *testing.T) {
	commands := []Command{
		{Name: "terraform", Commands: []Command{
			{Name: "build"},
		}},
		{Name: "docker"},
	}

	found, ok, ambiguous := FindCommandByName(commands, "build")
	assert.False(t, ambiguous)
	assert.True(t, ok)
	if assert.NotNil(t, found) {
		assert.Equal(t, "build", found.Name)
	}
}

func TestFindCommandByName_NotFound(t *testing.T) {
	commands := []Command{{Name: "docker"}}

	found, ok, ambiguous := FindCommandByName(commands, "missing")
	assert.False(t, ambiguous)
	assert.False(t, ok)
	assert.Nil(t, found)
}

// TestFindCommandByName_DuplicateNestedNamesAreAmbiguous guards against silently resolving
// dependencies.commands: [build] to whichever of two unrelated "build" commands the tree-walk
// happens to visit first -- that order is a declaration/merge-order implementation detail, not a
// meaningful disambiguation a config author actually chose.
func TestFindCommandByName_DuplicateNestedNamesAreAmbiguous(t *testing.T) {
	commands := []Command{
		{Name: "terraform", Commands: []Command{{Name: "build"}}},
		{Name: "docker", Commands: []Command{{Name: "build"}}},
	}

	found, ok, ambiguous := FindCommandByName(commands, "build")
	assert.True(t, ambiguous)
	assert.False(t, ok)
	assert.Nil(t, found)
}

// TestFindCommandByName_DuplicateTopLevelAndNestedNamesAreAmbiguous covers the mixed case: one
// top-level command and one nested command sharing a name.
func TestFindCommandByName_DuplicateTopLevelAndNestedNamesAreAmbiguous(t *testing.T) {
	commands := []Command{
		{Name: "build"},
		{Name: "terraform", Commands: []Command{{Name: "build"}}},
	}

	found, ok, ambiguous := FindCommandByName(commands, "build")
	assert.True(t, ambiguous)
	assert.False(t, ok)
	assert.Nil(t, found)
}
