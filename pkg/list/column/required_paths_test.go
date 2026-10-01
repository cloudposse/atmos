package column

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequiredPaths(t *testing.T) {
	for _, tc := range []struct {
		value string
		paths [][]string
	}{
		{"{{ .stack }}", [][]string{}},
		{"{{ .vars.literal }}", [][]string{{"vars", "literal"}}},
		{"{{ .settings.owner }} {{ .env.TEAM }}", [][]string{{"settings", "owner"}, {"env", "TEAM"}}},
		{"{{ .vars }}", [][]string{{"vars"}}},
		{"{{ index . .key }}", nil},
		{"{{ range .vars.items }}{{ .name }}{{ end }}", nil},
	} {
		t.Run(tc.value, func(t *testing.T) {
			assert.Equal(t, tc.paths, RequiredPaths([]Config{{Name: "test", Value: tc.value}}))
		})
	}
}
