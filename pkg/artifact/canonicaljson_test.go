package artifact

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCanonicalizeJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"sorts keys recursively", `{"b":{"z":1,"a":2},"a":[3,{"y":true,"x":null}]}`, `{"a":[3,{"x":null,"y":true}],"b":{"a":2,"z":1}}`},
		{"strips whitespace", " {\n \"a\" : [ 1 , 2 ] }\n", `{"a":[1,2]}`},
		{"minimal escapes", `{"s":"<>& \u007f"}`, "{\"s\":\"<>& \u007f\"}"},
		{"control characters use lowercase u00xx", `{"s":"\n\t\u001f"}`, `{"s":"\u000a\u0009\u001f"}`},
		{"quote and backslash", `{"s":"\"\\"}`, `{"s":"\"\\"}`},
		{"unicode literal", `{"s":"é é"}`, `{"s":"é é"}`},
		{"integers normalized", `[0,-0,-12,9223372036854775807]`, `[0,0,-12,9223372036854775807]`},
		{"scalar", `true`, `true`},
		{"empty containers", `{"a":{},"b":[]}`, `{"a":{},"b":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeJSON([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
			again, err := CanonicalizeJSON(got)
			require.NoError(t, err)
			assert.Equal(t, got, again, "canonicalization is idempotent")
		})
	}
}

func TestCanonicalizeJSONRejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"float", `{"a":1.5}`},
		{"exponent", `{"a":1e3}`},
		{"decimal zero", `{"a":1.0}`},
		{"integer overflow", `{"a":9223372036854775808}`},
		{"duplicate key", `{"a":1,"a":2}`},
		{"nested duplicate key", `{"x":{"a":1,"a":1}}`},
		{"invalid utf8", "{\"a\":\"\xff\"}"},
		{"lone surrogate escape", `{"a":"\ud800"}`},
		{"replacement character", "{\"a\":\"�\"}"},
		{"trailing data", `{"a":1} {"b":2}`},
		{"trailing garbage", `{"a":1} x`},
		{"truncated", `{"a":`},
		{"empty", ``},
		{"deep nesting", strings.Repeat("[", maxJSONDepth+2) + strings.Repeat("]", maxJSONDepth+2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CanonicalizeJSON([]byte(tt.input))
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid)
		})
	}
}

func TestReadBounded(t *testing.T) {
	data, err := ReadBounded(strings.NewReader("12345"), 5)
	require.NoError(t, err)
	assert.Equal(t, "12345", string(data))

	_, err = ReadBounded(strings.NewReader("123456"), 5)
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)

	boom := errors.New("boom")
	_, err = ReadBounded(io.MultiReader(strings.NewReader("1"), errReader{boom}), 5)
	require.ErrorIs(t, err, boom)
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestReadRecord(t *testing.T) {
	record, raw, err := ReadRecord(strings.NewReader(vectorACanonical))
	require.NoError(t, err)
	assert.Equal(t, vectorACanonical, string(raw))
	assert.Equal(t, "components/terraform/vpc", record.Name)

	_, _, err = ReadRecord(bytes.NewReader(make([]byte, MaxRecordSize+1)))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)

	_, _, err = ReadRecord(strings.NewReader("{}"))
	require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid)
}
