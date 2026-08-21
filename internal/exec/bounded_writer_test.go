package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBoundedWriterPanics(t *testing.T) {
	t.Run("panics on zero capacity", func(t *testing.T) {
		assert.Panics(t, func() { NewBoundedWriter(0) })
	})
	t.Run("panics on negative capacity", func(t *testing.T) {
		assert.Panics(t, func() { NewBoundedWriter(-1) })
	})
}

func TestBoundedWriter(t *testing.T) {
	t.Run("nothing written returns nil Bytes and not truncated", func(t *testing.T) {
		w := NewBoundedWriter(10)
		assert.Nil(t, w.Bytes())
		assert.False(t, w.Truncated())
	})

	t.Run("write less than capacity keeps all bytes in order", func(t *testing.T) {
		w := NewBoundedWriter(10)
		n, err := w.Write([]byte("hello"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), w.Bytes())
		assert.False(t, w.Truncated())
	})

	t.Run("write exactly capacity keeps all bytes", func(t *testing.T) {
		w := NewBoundedWriter(5)
		n, err := w.Write([]byte("ABCDE"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("ABCDE"), w.Bytes())
		assert.False(t, w.Truncated())
	})

	t.Run("write exceeds capacity keeps tail", func(t *testing.T) {
		w := NewBoundedWriter(5)
		_, _ = w.Write([]byte("ABCDEFGH")) // 8 bytes > capacity 5
		assert.Equal(t, []byte("DEFGH"), w.Bytes())
		assert.True(t, w.Truncated())
	})

	t.Run("multiple small writes accumulate below capacity", func(t *testing.T) {
		w := NewBoundedWriter(10)
		_, _ = w.Write([]byte("foo"))
		_, _ = w.Write([]byte("bar"))
		assert.Equal(t, []byte("foobar"), w.Bytes())
		assert.False(t, w.Truncated())
	})

	t.Run("multiple writes that wrap ring keep correct tail", func(t *testing.T) {
		w := NewBoundedWriter(5)
		_, _ = w.Write([]byte("ABCDE")) // fills ring: pos=0, not truncated yet
		_, _ = w.Write([]byte("FGH"))   // wraps: total=8
		assert.Equal(t, []byte("DEFGH"), w.Bytes())
		assert.True(t, w.Truncated())
	})

	t.Run("single write larger than capacity", func(t *testing.T) {
		w := NewBoundedWriter(5)
		_, _ = w.Write([]byte("ABCDEFGHIJKLMNOP")) // 16 bytes
		assert.Equal(t, []byte("LMNOP"), w.Bytes())
		assert.True(t, w.Truncated())
	})

	t.Run("subsequent write after oversized write", func(t *testing.T) {
		w := NewBoundedWriter(5)
		_, _ = w.Write([]byte("ABCDEFGHIJKLMNOP")) // leaves ring=[L,M,N,O,P], pos=0
		_, _ = w.Write([]byte("QR"))               // appends QR
		assert.Equal(t, []byte("NOPQR"), w.Bytes())
		assert.True(t, w.Truncated())
	})

	t.Run("empty write is a no-op", func(t *testing.T) {
		w := NewBoundedWriter(5)
		_, _ = w.Write([]byte("ABC"))
		n, err := w.Write([]byte{})
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, []byte("ABC"), w.Bytes())
	})

	t.Run("Bytes returns a copy not a slice alias", func(t *testing.T) {
		w := NewBoundedWriter(10)
		_, _ = w.Write([]byte("hello"))
		b := w.Bytes()
		b[0] = 'X'
		assert.Equal(t, []byte("hello"), w.Bytes())
	})

	t.Run("large realistic output keeps last N bytes", func(t *testing.T) {
		const cap = 3 * 1024 * 1024 // 3 MB
		w := NewBoundedWriter(cap)
		head := make([]byte, cap+100)
		for i := range head {
			head[i] = 'A'
		}
		tail := []byte("Plan: 5 to add, 0 to change, 0 to destroy.")
		// Write head then tail to simulate large terraform output.
		_, _ = w.Write(head)
		_, _ = w.Write(tail)
		got := w.Bytes()
		require.True(t, w.Truncated())
		// Tail must be present at the end.
		assert.Equal(t, tail, got[len(got)-len(tail):])
	})
}
