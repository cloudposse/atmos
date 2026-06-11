package exec

// BoundedWriter is a write-time bounded io.Writer that retains only the last
// capacity bytes ever written. It prevents unbounded memory growth when capturing
// large command output (e.g. terraform plan/apply) while preserving the most
// useful tail content (plan summary, apply results, error messages).
//
// Internally it is a fixed-size ring buffer; once the buffer is full, older
// bytes are silently overwritten by newer ones on every Write call.
//
// The zero value is unusable; construct with NewBoundedWriter.
type BoundedWriter struct {
	ring     []byte // ring buffer of exactly capacity bytes
	written  int    // total bytes written across all Write calls
	pos      int    // next write index (wraps modulo capacity)
	capacity int
}

// NewBoundedWriter returns a BoundedWriter that keeps at most maxBytes bytes of
// tail output.  maxBytes must be positive.
func NewBoundedWriter(maxBytes int) *BoundedWriter {
	return &BoundedWriter{
		ring:     make([]byte, maxBytes),
		capacity: maxBytes,
	}
}

// Write implements io.Writer.  It always consumes all of p and never returns an
// error.  When the total bytes written exceed capacity, only the most-recent
// capacity bytes are retained.
func (w *BoundedWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.written += n

	if n == 0 {
		return 0, nil
	}

	if n >= w.capacity {
		// p alone fills or exceeds the ring; keep only the last capacity bytes
		// and reset pos to 0 so Bytes() can read the ring in order.
		copy(w.ring, p[n-w.capacity:])
		w.pos = 0
		return n, nil
	}

	end := w.pos + n
	if end <= w.capacity {
		copy(w.ring[w.pos:], p)
	} else {
		// p wraps around the ring boundary.
		firstPart := w.capacity - w.pos
		copy(w.ring[w.pos:], p[:firstPart])
		copy(w.ring, p[firstPart:])
	}
	w.pos = end % w.capacity
	return n, nil
}

// Bytes returns a copy of the buffered content in write order.
// The length is min(total-written, capacity).  Returns nil when nothing has
// been written yet.
func (w *BoundedWriter) Bytes() []byte {
	if w.written == 0 {
		return nil
	}
	if w.written <= w.capacity {
		// Ring has not wrapped; all data lives at ring[0:written].
		return append([]byte(nil), w.ring[:w.written]...)
	}
	// Ring has wrapped: ordered content runs ring[pos:] then ring[:pos].
	out := make([]byte, w.capacity)
	n := copy(out, w.ring[w.pos:])
	copy(out[n:], w.ring[:w.pos])
	return out
}

// Truncated reports whether total bytes written exceeded the buffer capacity,
// meaning some leading output was silently dropped.
func (w *BoundedWriter) Truncated() bool {
	return w.written > w.capacity
}
