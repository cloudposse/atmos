package io

import (
	"os"
	"strings"
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
	if strings.Contains(rec.data, "super-secret") {
		t.Fatal("recorder received unmasked output")
	}
	if !strings.Contains(rec.data, MaskReplacement) {
		t.Fatalf("recorder did not receive mask replacement: %q", rec.data)
	}
}

func TestRecordMarker(t *testing.T) {
	tests := []struct {
		name       string
		install    bool
		label      string
		wantStream string
		wantData   string
	}{
		{name: "records label on marker stream", install: true, label: "checkpoint-1", wantStream: "m", wantData: "checkpoint-1"},
		{name: "empty label is ignored", install: true, label: "", wantStream: "", wantData: ""},
		{name: "no recorder installed is a no-op", install: false, label: "checkpoint-1", wantStream: "", wantData: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &testRecorder{}
			if tt.install {
				restore := SetRecorder(rec)
				defer restore()
			} else {
				restore := SetRecorder(nil)
				defer restore()
			}

			RecordMarker(tt.label)

			if rec.stream != tt.wantStream {
				t.Errorf("stream = %q, want %q", rec.stream, tt.wantStream)
			}
			if rec.data != tt.wantData {
				t.Errorf("data = %q, want %q", rec.data, tt.wantData)
			}
		})
	}
}
