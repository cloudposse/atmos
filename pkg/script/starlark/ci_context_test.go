package starlark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
)

// Compile-time sentinel: a rename of the pull request fork field must fail the build here.
var _ = ci.PRInfo{Number: 0, HeadRef: "", BaseRef: "", URL: "", Fork: false}

func TestCIContextPullRequestFork(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ctx    *ci.Context
		source string
		want   string
		asJSON bool
	}{
		{
			name:   "fork pull request",
			ctx:    &ci.Context{Provider: "github-actions", PullRequest: &ci.PRInfo{Number: 5, HeadRef: "feat", BaseRef: "main", Fork: true}},
			source: `print(ci.context.pr.fork, ci.context.pr.number)`,
			want:   "True 5\n",
		},
		{
			name:   "same-repository pull request",
			ctx:    &ci.Context{Provider: "github-actions", PullRequest: &ci.PRInfo{Number: 6, HeadRef: "feat", BaseRef: "main"}},
			source: `print(ci.context.pr.fork, ci.context.pr.number)`,
			want:   "False 6\n",
		},
		{
			name:   "fork is encoded with the other pull request fields",
			ctx:    &ci.Context{Provider: "github-actions", PullRequest: &ci.PRInfo{Number: 7, HeadRef: "h", BaseRef: "b", URL: "https://pr", Fork: true}},
			source: `print(json.encode(ci.context.pr))`,
			want:   `{"number":7,"head":"h","base":"b","url":"https://pr","fork":true}`,
			asJSON: true,
		},
		{
			name:   "no pull request",
			ctx:    &ci.Context{Provider: "github-actions", EventName: "push"},
			source: `print(ci.context.pr)`,
			want:   "None\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			m.EXPECT().Context().Return(tt.ctx, nil)

			stdout, _, err := runCI(t, m, tt.source)

			require.NoError(t, err)
			if tt.asJSON {
				assert.JSONEq(t, tt.want, stdout)
				return
			}
			assert.Equal(t, tt.want, stdout)
		})
	}
}
