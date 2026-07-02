package io

import (
	"os"
	"testing"
)

type testRecorder struct {
	stream string
	data   string
}

func (r *testRecorder) Record(stream, content string) {
	r.stream = stream
	r.data += content
}

func TestContextWriteRecordsMaskedOutput(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()

	rec := &testRecorder{}
	restore := SetRecorder(rec)
	defer restore()

	ctx, err := NewContext()
	if err != nil {
		t.Fatal(err)
	}
	ctx.Masker().RegisterSecret("super-secret")
	if err := ctx.Write(DataStream, "value=super-secret"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	_ = reader.Close()

	if rec.stream != "o" {
		t.Fatalf("expected stdout stream, got %q", rec.stream)
	}
	if rec.data == "value=super-secret" {
		t.Fatal("recorder received unmasked output")
	}
}
