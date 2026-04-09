package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDeferredMergeContext(t *testing.T) {
	t.Run("creates context with empty deferred values", func(t *testing.T) {
		dctx := NewDeferredMergeContext()

		assert.NotNil(t, dctx)
		assert.NotNil(t, dctx.deferredValues)
		assert.Equal(t, 0, dctx.precedence)
		assert.False(t, dctx.HasDeferredValues())
	})
}

func TestDeferredMergeContext_AddDeferred(t *testing.T) {
	t.Run("adds deferred value with correct path and precedence", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		path := []string{"components", "terraform", "vpc", "vars", "config"}
		value := "!template '{{ .settings.base }}'"

		dctx.AddDeferred(path, value)

		assert.True(t, dctx.HasDeferredValues())
		values := dctx.GetDeferredValues()
		assert.Len(t, values, 1)

		key := "components.terraform.vpc.vars.config"
		assert.Contains(t, values, key)
		assert.Len(t, values[key], 1)

		dv := values[key][0]
		assert.Equal(t, path, dv.Path)
		assert.Equal(t, value, dv.Value)
		assert.Equal(t, 0, dv.Precedence)
		assert.True(t, dv.IsFunction)
	})

	t.Run("adds multiple values for same path", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		path := []string{"vars", "config"}

		dctx.AddDeferred(path, "!template 'value1'")
		dctx.IncrementPrecedence()
		dctx.AddDeferred(path, "!template 'value2'")

		values := dctx.GetDeferredValues()
		key := "vars.config"
		assert.Len(t, values[key], 2)
		assert.Equal(t, 0, values[key][0].Precedence)
		assert.Equal(t, 1, values[key][1].Precedence)
	})

	t.Run("adds values with different paths", func(t *testing.T) {
		dctx := NewDeferredMergeContext()

		dctx.AddDeferred([]string{"vars", "foo"}, "!template 'foo'")
		dctx.AddDeferred([]string{"vars", "bar"}, "!template 'bar'")

		values := dctx.GetDeferredValues()
		assert.Len(t, values, 2)
		assert.Contains(t, values, "vars.foo")
		assert.Contains(t, values, "vars.bar")
	})
}

func TestDeferredMergeContext_IncrementPrecedence(t *testing.T) {
	t.Run("increments precedence counter", func(t *testing.T) {
		dctx := NewDeferredMergeContext()

		assert.Equal(t, 0, dctx.precedence)

		dctx.IncrementPrecedence()
		assert.Equal(t, 1, dctx.precedence)

		dctx.IncrementPrecedence()
		assert.Equal(t, 2, dctx.precedence)
	})

	t.Run("affects precedence of added values", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		path := []string{"vars", "test"}

		dctx.AddDeferred(path, "value1")
		dctx.IncrementPrecedence()
		dctx.AddDeferred(path, "value2")
		dctx.IncrementPrecedence()
		dctx.AddDeferred(path, "value3")

		values := dctx.GetDeferredValues()["vars.test"]
		assert.Equal(t, 0, values[0].Precedence)
		assert.Equal(t, 1, values[1].Precedence)
		assert.Equal(t, 2, values[2].Precedence)
	})
}

func TestDeferredMergeContext_HasDeferredValues(t *testing.T) {
	t.Run("returns false when empty", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("returns true when values exist", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"test"}, "value")
		assert.True(t, dctx.HasDeferredValues())
	})
}

func TestDeferredMergeContext_GetDeferredValues(t *testing.T) {
	t.Run("returns empty map initially", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		values := dctx.GetDeferredValues()
		assert.NotNil(t, values)
		assert.Len(t, values, 0)
	})

	t.Run("returns all deferred values", func(t *testing.T) {
		dctx := NewDeferredMergeContext()

		dctx.AddDeferred([]string{"path1"}, "value1")
		dctx.AddDeferred([]string{"path2"}, "value2")
		dctx.AddDeferred([]string{"path1"}, "value3")

		values := dctx.GetDeferredValues()
		assert.Len(t, values, 2)
		assert.Len(t, values["path1"], 2)
		assert.Len(t, values["path2"], 1)
	})
}
