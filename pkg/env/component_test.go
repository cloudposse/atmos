package env

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrepareComponentEnv(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		section  map[string]any
		existing []string
		want     []string
	}{
		{name: "empty"},
		{name: "retain existing", existing: []string{"BASE=kept"}, want: []string{"BASE=kept"}},
		{
			name:     "append formatted values",
			section:  map[string]any{"TEXT": "a=b", "NUMBER": 42, "BOOL": false, "EMPTY": ""},
			existing: []string{"BASE=kept"},
			want:     []string{"BASE=kept", "TEXT=a=b", "NUMBER=42", "BOOL=false", "EMPTY="},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := schema.ConfigAndStacksInfo{ComponentEnvSection: tc.section, ComponentEnvList: tc.existing}
			PrepareComponentEnv(&info)
			assert.ElementsMatch(t, tc.want, info.ComponentEnvList)
			assert.Equal(t, tc.section, info.ComponentEnvSection)
		})
	}
}
