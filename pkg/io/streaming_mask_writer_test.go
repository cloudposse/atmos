package io

import (
	"bytes"
	"encoding/base64"
	"errors"
	stdio "io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingWriter records every Write call so tests can assert exactly when output is emitted.
type recordingWriter struct {
	mu     sync.Mutex
	writes []string
}

func (r *recordingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, string(p))
	return len(p), nil
}

func (r *recordingWriter) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.writes, "")
}

func newTestStreamingWriter(m Masker, w *recordingWriter, opts ...StreamingMaskOption) *StreamingMaskWriter {
	return NewStreamingMaskWriter(w, append([]StreamingMaskOption{withStreamingMasker(m)}, opts...)...)
}

// streamInChunks writes input split at the given offsets, flushes, and returns what was emitted.
func streamInChunks(t *testing.T, m Masker, input string, offsets ...int) string {
	t.Helper()
	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	prev := 0
	for _, off := range append(offsets, len(input)) {
		n, err := sw.Write([]byte(input[prev:off]))
		require.NoError(t, err)
		require.Equal(t, off-prev, n)
		prev = off
	}
	require.NoError(t, sw.Flush())
	return rec.String()
}

func TestStreamingMaskWriter_SingleLineSecretSplitAtEveryOffset(t *testing.T) {
	const secret = "s3cr3t-token-value"
	m := newMasker(&Config{})
	m.RegisterSecret(secret)

	inputs := map[string]string{
		"plain":      "before " + secret + " after\n",
		"at start":   secret + " after",
		"at end":     "before " + secret,
		"repeated":   secret + secret + " " + secret,
		"base64":     "token=" + base64.StdEncoding.EncodeToString([]byte(secret)) + "\n",
		"json-quote": `{"k":"` + secret + `"}`,
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			want := m.Mask(input)
			require.NotContains(t, want, secret)

			for k := 0; k <= len(input); k++ {
				got := streamInChunks(t, m, input, k)
				assert.NotContains(t, got, secret, "split at %d leaked", k)
				assert.Equal(t, want, got, "split at %d", k)
			}
		})
	}
}

func TestStreamingMaskWriter_SecretSplitAcrossThreeWrites(t *testing.T) {
	const literal = "atmos-split-value"
	m := newMasker(&Config{})
	m.RegisterSecret(literal)
	input := "x " + literal + " y " + literal

	want := m.Mask(input)
	for i := 0; i <= len(input); i++ {
		for j := i; j <= len(input); j++ {
			got := streamInChunks(t, m, input, i, j)
			require.Equal(t, want, got, "splits at %d,%d", i, j)
		}
	}
}

func TestStreamingMaskWriter_ByteAtATime(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret(testPEM)
	m.RegisterSecret("plain-token-1")
	input := "start\n" + testPEM + "mid plain-token-1 end\n"

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	for i := 0; i < len(input); i++ {
		_, err := sw.Write([]byte{input[i]})
		require.NoError(t, err)
	}
	require.NoError(t, sw.Flush())
	assert.Equal(t, m.Mask(input), rec.String())
	assert.NotContains(t, rec.String(), "plain-token-1")
}

func TestStreamingMaskWriter_MultilineSecretSplitAcrossBatches(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret(testPEM)

	inputs := map[string]string{
		"verbatim":    "before\n" + testPEM + "after\n",
		"yaml-indent": "key: |\n    -----BEGIN TEST CERTIFICATE-----\n    ATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA\n    ATMOS-TEST-PEM-LINE-TWO-BBBBBBBBBBBBBBBB\n    -----END TEST CERTIFICATE-----\nnext: 1\n",
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			want := m.Mask(input)
			require.Contains(t, want, MaskReplacement)
			for _, line := range []string{"ATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA", "ATMOS-TEST-PEM-LINE-TWO-BBBBBBBBBBBBBBBB"} {
				require.NotContains(t, want, line)
			}

			for k := 0; k <= len(input); k++ {
				got := streamInChunks(t, m, input, k)
				assert.Equal(t, want, got, "split at %d", k)
			}
		})
	}
}

func TestStreamingMaskWriter_PEMSplitAtLineBatchesCollapsesLikeUnsplit(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret(testPEM)

	lines := strings.SplitAfter(testPEM, "\n")
	// Batches of complete lines, as a line-oriented forwarder would deliver them.
	for batch := 1; batch < len(lines); batch++ {
		rec := &recordingWriter{}
		sw := newTestStreamingWriter(m, rec)
		_, err := sw.Write([]byte("header\n" + strings.Join(lines[:batch], "")))
		require.NoError(t, err)
		_, err = sw.Write([]byte(strings.Join(lines[batch:], "") + "footer\n"))
		require.NoError(t, err)
		require.NoError(t, sw.Flush())

		want := m.Mask("header\n" + testPEM + "footer\n")
		assert.Equal(t, want, rec.String(), "batch boundary after %d lines", batch)
		assert.Equal(t, 1, strings.Count(rec.String(), MaskReplacement), "PEM collapses to one mask")
	}
}

func TestStreamingMaskWriter_OrdinaryOutputIsNotHeldBack(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret("s3cr3t-token-value")

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)

	for _, chunk := range []string{"hello ", "world\n", "Enter a value: ", "no secret prefix here"} {
		before := len(rec.writes)
		n, err := sw.Write([]byte(chunk))
		require.NoError(t, err)
		assert.Equal(t, len(chunk), n)
		require.Len(t, rec.writes, before+1, "chunk %q must be emitted on the same Write", chunk)
		assert.Equal(t, chunk, rec.writes[len(rec.writes)-1])
	}
}

func TestStreamingMaskWriter_HoldsOnlyTheSecretPrefix(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret("s3cr3t-token-value")

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)

	_, err := sw.Write([]byte("visible s3cr3"))
	require.NoError(t, err)
	assert.Equal(t, "visible ", rec.String(), "only the possible secret prefix is withheld")

	_, err = sw.Write([]byte("t-token-value!"))
	require.NoError(t, err)
	assert.Equal(t, "visible "+MaskReplacement+"!", rec.String())
}

func TestStreamingMaskWriter_ReleasesHeldBytesThatTurnOutHarmless(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret("s3cr3t-token-value")

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	_, _ = sw.Write([]byte("see s3cr3"))
	_, _ = sw.Write([]byte("amble and more"))
	assert.Equal(t, "see s3cr3amble and more", rec.String(), "a non-matching continuation releases the held tail")
}

func TestStreamingMaskWriter_FlushEmitsHeldTailMasked(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterValue("abc")
	m.RegisterValue("abcdef")

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	_, _ = sw.Write([]byte("x abc"))
	assert.Equal(t, "x ", rec.String(), "abc may still become abcdef")

	require.NoError(t, sw.Flush())
	assert.Equal(t, "x "+MaskReplacement, rec.String(), "flush masks the held tail")

	before := len(rec.writes)
	require.NoError(t, sw.Flush())
	assert.Len(t, rec.writes, before, "a second flush with nothing held writes nothing")
}

func TestStreamingMaskWriter_NeverSplitsCompleteOccurrence(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterValue("abc")
	m.RegisterValue("bcdef")

	// "bcd" is a prefix of bcdef, but cutting before it would split the complete "abc".
	got := streamInChunks(t, m, "zabcd", 5)
	assert.Equal(t, m.Mask("zabcd"), got)

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	_, _ = sw.Write([]byte("zabcd"))
	assert.Equal(t, "z", rec.String(), "cut moved before the complete occurrence it would split")
}

func TestStreamingMaskWriter_RegexPatternsHoldUnfinishedLine(t *testing.T) {
	m := newMasker(&Config{})
	require.NoError(t, m.RegisterPattern(`Bearer [A-Za-z0-9]+`))

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	_, _ = sw.Write([]byte("done\nAuthorization: Bear"))
	assert.Equal(t, "done\n", rec.String(), "unfinished line is held while regex patterns exist")
	_, _ = sw.Write([]byte("er abc123\nnext"))
	assert.Equal(t, "done\nAuthorization: "+MaskReplacement+"\n", rec.String())
	require.NoError(t, sw.Flush())
	assert.Equal(t, "done\nAuthorization: "+MaskReplacement+"\nnext", rec.String())

	// A carriage return (progress output) also ends a line.
	rec = &recordingWriter{}
	sw = newTestStreamingWriter(m, rec)
	_, _ = sw.Write([]byte("10%\r20%\rpartial"))
	assert.Equal(t, "10%\r20%\r", rec.String())
}

func TestStreamingMaskWriter_LongRegexSecrets(t *testing.T) {
	payload := strings.Repeat("x", 12*1024)
	tests := []struct {
		name    string
		pattern string
		chunks  []string
	}{
		{
			name:    "long token before newline",
			pattern: `Bearer [A-Za-z0-9]+`,
			chunks:  []string{"Bearer " + payload, "\n"},
		},
		{
			name:    "split prefix and multiple token chunks",
			pattern: `Bearer [A-Za-z0-9]+`,
			chunks:  []string{"Bear", "er " + payload[:5000], payload[5000:9000], payload[9000:], "\n"},
		},
		{
			name:    "flush without newline",
			pattern: `Bearer [A-Za-z0-9]+`,
			chunks:  []string{"Bearer " + payload[:7000], payload[7000:]},
		},
		{
			name:    "pattern matches only after its closing delimiter",
			pattern: `BEGIN [A-Za-z0-9]+ END`,
			chunks:  []string{"BEGIN " + payload[:5000], payload[5000:], " END", "\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMasker(&Config{})
			require.NoError(t, m.RegisterPattern(tt.pattern))
			rec := &recordingWriter{}
			sw := newTestStreamingWriter(m, rec)
			_, err := sw.Write([]byte("ordinary output\n"))
			require.NoError(t, err)
			for _, chunk := range tt.chunks {
				n, err := sw.Write([]byte(chunk))
				require.NoError(t, err)
				require.Equal(t, len(chunk), n)
				require.Zero(t, strings.Count(rec.String(), "x"), "no token bytes may be emitted, even before Flush")
			}
			require.NoError(t, sw.Flush())
			want := "ordinary output\n" + MaskReplacement
			if strings.HasSuffix(tt.chunks[len(tt.chunks)-1], "\n") {
				want += "\n"
			}
			assert.Equal(t, want, rec.String())
		})
	}
}

func TestStreamingMaskWriter_WithoutLineHold(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterValue("s3cr3t-token-value")
	require.NoError(t, m.RegisterPattern(`Bearer [A-Za-z0-9]+`))

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec, WithoutLineHold())
	_, _ = sw.Write([]byte("Enter a value: "))
	assert.Equal(t, "Enter a value: ", rec.String(), "prompts without a newline are shown immediately")

	_, _ = sw.Write([]byte("s3cr3t-tok"))
	_, _ = sw.Write([]byte("en-value"))
	assert.Equal(t, "Enter a value: "+MaskReplacement, rec.String(), "literals stay protected")
}

func TestStreamingMaskWriter_DisabledMaskingPassesThrough(t *testing.T) {
	m := newMasker(&Config{DisableMasking: true})
	m.RegisterValue("s3cr3t-token-value")

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	_, _ = sw.Write([]byte("s3cr3t-tok"))
	assert.Equal(t, "s3cr3t-tok", rec.String())
}

func TestStreamingMaskWriter_NilMaskerPassesThrough(t *testing.T) {
	var buf bytes.Buffer
	sw := &StreamingMaskWriter{underlying: &buf}
	n, err := sw.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String())
}

type failingWriter struct {
	err   error
	short bool
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.short {
		return len(p) / 2, nil
	}
	return 0, f.err
}

func TestStreamingMaskWriter_PropagatesWriteErrors(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterValue("s3cr3t-token-value")

	boom := errors.New("boom")
	sw := NewStreamingMaskWriter(&failingWriter{err: boom}, withStreamingMasker(m))
	n, err := sw.Write([]byte("hello"))
	require.ErrorIs(t, err, boom)
	assert.Zero(t, n)

	sw = NewStreamingMaskWriter(&failingWriter{short: true}, withStreamingMasker(m))
	_, err = sw.Write([]byte("hello"))
	require.ErrorIs(t, err, stdio.ErrShortWrite)

	sw = NewStreamingMaskWriter(&failingWriter{err: boom}, withStreamingMasker(m))
	_, err = sw.Write([]byte("s3cr3t"))
	require.NoError(t, err, "held tail is not written yet")
	require.ErrorIs(t, sw.Flush(), boom)
}

func TestStreamingMaskWriter_ConcurrentWriters(t *testing.T) {
	const secret = "concurrent-secret-value"
	m := newMasker(&Config{})
	m.RegisterSecret(secret)

	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)

	const writers, iterations = 8, 200
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			// Each logical record is written in a single call so records never interleave mid-secret.
			for range iterations {
				_, err := sw.Write([]byte("line " + secret + "\n"))
				assert.NoError(t, err)
				_, err = sw.Write([]byte("ordinary\n"))
				assert.NoError(t, err)
			}
		})
	}
	wg.Wait()
	require.NoError(t, sw.Flush())

	out := rec.String()
	assert.NotContains(t, out, secret)
	assert.Equal(t, writers*iterations, strings.Count(out, "line "+MaskReplacement+"\n"))
	assert.Equal(t, writers*iterations, strings.Count(out, "ordinary\n"))
}

func TestStreamingMaskWriter_ConcurrentWithRegistration(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret("initial-secret-value")
	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 200 {
			m.RegisterValue("another-secret-" + strings.Repeat("x", 5))
			m.RegisterSecret("late-secret-value")
		}
	})
	wg.Go(func() {
		for range 200 {
			_, _ = sw.Write([]byte("initial-sec"))
			_, _ = sw.Write([]byte("ret-value\n"))
		}
	})
	wg.Wait()
	require.NoError(t, sw.Flush())
	assert.NotContains(t, rec.String(), "initial-secret-value")
}

func TestNewStreamingMaskWriter_UsesGlobalMasker(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	require.NoError(t, Initialize())
	RegisterValue("global-streamed-secret")

	var buf bytes.Buffer
	sw := NewStreamingMaskWriter(&buf)
	_, err := sw.Write([]byte("a global-streamed"))
	require.NoError(t, err)
	_, err = sw.Write([]byte("-secret b"))
	require.NoError(t, err)
	require.NoError(t, sw.Flush())
	assert.Equal(t, "a "+MaskReplacement+" b", buf.String())
}

func BenchmarkStreamingMaskWriter(b *testing.B) {
	m := newMasker(&Config{})
	for _, secret := range []string{"alpha-secret-token", "beta-secret-token", testPEM} {
		m.RegisterSecret(secret)
	}
	chunk := []byte(strings.Repeat("ordinary terraform output line without secrets in it\n", 20))

	b.Run("streaming", func(b *testing.B) {
		sw := NewStreamingMaskWriter(&recordingWriter{}, withStreamingMasker(m))
		b.SetBytes(int64(len(chunk)))
		b.ReportAllocs()
		for b.Loop() {
			_, _ = sw.Write(chunk)
		}
	})
	b.Run("per-write", func(b *testing.B) {
		w := &maskedWriter{underlying: &recordingWriter{}, masker: m}
		b.SetBytes(int64(len(chunk)))
		b.ReportAllocs()
		for b.Loop() {
			_, _ = w.Write(chunk)
		}
	})
}

func TestStreamingMaskWriter_BoundedCrossLineRegex(t *testing.T) {
	tests := []struct{ name, pattern, secret string }{
		{"newline", `secret\nvalue`, "secret\nvalue"},
		{"carriage return", `secret\rvalue`, "secret\rvalue"},
		{"CRLF", `secret\r\nvalue`, "secret\r\nvalue"},
		{"class", `secret[\s]value`, "secret\nvalue"},
		{"dot", `secret.value`, "secret\rvalue"},
		{"dot all repetition", `(?s)secret.{1,3}value`, "secret\r\nvalue"},
		{"negated class", `secret[^x]{1,2}value`, "secret\nvalue"},
		{"unicode and alternation", `(?:秘密|x)\n[🌍🌎]{1,2}`, "秘密\n🌍🌎"},
		{"case folded unicode", `(?i)k\nvalue`, "K\nvalue"},
		{"line anchors", `(?m)^secret\nvalue$`, "secret\nvalue"},
		{"word boundaries", `\bsecret\nvalue\b`, "secret\nvalue"},
		{"optional segment", `secret(?:\r)?\nvalue`, "secret\nvalue"},
		{"Unicode literal widths", `é秘🌍\nvalue`, "é秘🌍\nvalue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMasker(&Config{})
			require.NoError(t, m.RegisterPattern(tt.pattern))
			input := "safe line\n" + tt.secret + "\n" + strings.Repeat("after\n", 20)
			want := "safe line\n" + MaskReplacement + "\n" + strings.Repeat("after\n", 20)
			for split := 0; split <= len(input); split++ {
				assert.Equal(t, want, streamInChunks(t, m, input, split), "split offset %d", split)
			}
			rec := &recordingWriter{}
			sw := newTestStreamingWriter(m, rec)
			for _, b := range []byte(input) {
				_, err := sw.Write([]byte{b})
				require.NoError(t, err)
			}
			assert.Contains(t, rec.String(), MaskReplacement, "a complete secret is masked before Flush")
			assert.Contains(t, rec.String(), "after\n", "completed safe output is released before Flush")
			require.NoError(t, sw.Flush())
			assert.Equal(t, want, rec.String())
		})
	}
}

func TestStreamingMaskWriter_MixedCrossLineAndUnboundedLinePatterns(t *testing.T) {
	m := newMasker(&Config{})
	require.NoError(t, m.RegisterPattern(`secret\nvalue`))
	require.NoError(t, m.RegisterPattern(`BEGIN [a-z]+ END`))
	payload := strings.Repeat("x", 12*1024)
	rec := &recordingWriter{}
	sw := newTestStreamingWriter(m, rec)
	for _, part := range []string{"ready\n", "BEGIN " + payload[:5000], payload[5000:], " END\nsecret\n", "value\n", strings.Repeat("done\n", 20)} {
		_, err := sw.Write([]byte(part))
		require.NoError(t, err)
		assert.NotContains(t, rec.String(), "x")
		assert.NotContains(t, rec.String(), "secret")
	}
	assert.Contains(t, rec.String(), "done\n", "bounded cross-line support must keep streaming")
	require.NoError(t, sw.Flush())
	assert.Equal(t, "ready\n"+MaskReplacement+"\n"+MaskReplacement+"\n"+strings.Repeat("done\n", 20), rec.String())
}
