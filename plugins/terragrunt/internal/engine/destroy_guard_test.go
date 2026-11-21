package engine

import (
	"errors"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestStaticDestroyGuardFailsClosed(t *testing.T) {
	t.Parallel()
	parseErr := errors.New("evaluated guard unavailable")
	for _, test := range []struct {
		name, content string
		missing       bool
		want          error
	}{
		{name: "literal true", content: "prevent_destroy = true", want: ErrPreventDestroy},
		{name: "literal false", content: "prevent_destroy = false"},
		{name: "absent", content: "locals { value = 1 }"},
		{name: "included protection cannot be ruled out", content: "include \"base\" { path = \"base.hcl\" }\nprevent_destroy = false", want: parseErr},
		{name: "expression", content: "prevent_destroy = local.protected", want: parseErr},
		{name: "null", content: "prevent_destroy = null", want: parseErr},
		{name: "non boolean", content: "prevent_destroy = \"false\"", want: parseErr},
		{name: "malformed", content: "prevent_destroy = [", want: parseErr},
		{name: "unreadable", missing: true, want: parseErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fs := afero.NewMemMapFs()
			if !test.missing {
				require.NoError(t, afero.WriteFile(fs, "terragrunt.hcl", []byte(test.content), 0o600))
			}
			err := staticDestroyGuard(fs, "terragrunt.hcl", parseErr)
			if test.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.want)
			}
		})
	}
}
