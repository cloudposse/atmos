package target

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeYAMLDocuments(t *testing.T) {
	tests := []struct {
		name string
		docs [][]byte
		want string
	}{
		{"empty", nil, ""},
		{"single document", [][]byte{[]byte("kind: Namespace\n")}, "kind: Namespace\n"},
		{
			"multiple documents joined with separator",
			[][]byte{[]byte("kind: Namespace\n"), []byte("kind: Deployment\n")},
			"kind: Namespace\n---\nkind: Deployment\n",
		},
		{
			"trailing newline is normalized when missing",
			[][]byte{[]byte("kind: Namespace"), []byte("kind: Deployment")},
			"kind: Namespace\n---\nkind: Deployment\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(MergeYAMLDocuments(tt.docs)))
		})
	}
}
