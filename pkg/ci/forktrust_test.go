package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateForkCheckout(t *testing.T) {
	const baseRepo = "acme/infra"

	tests := []struct {
		name          string
		ctx           *Context
		req           CloneRequest
		wantUntrusted bool
	}{
		{
			name:          "nil context is trusted",
			ctx:           nil,
			req:           CloneRequest{Ref: "refs/pull/42/merge"},
			wantUntrusted: false,
		},
		{
			name:          "non-elevated event with PR ref is trusted",
			ctx:           &Context{EventName: "pull_request", ElevatedEvent: false, Repository: baseRepo},
			req:           CloneRequest{Ref: "refs/pull/42/merge"},
			wantUntrusted: false,
		},
		{
			name:          "non-elevated event with cross-repo URI is trusted",
			ctx:           &Context{EventName: "push", ElevatedEvent: false, Repository: baseRepo},
			req:           CloneRequest{URI: "https://github.com/attacker/infra.git"},
			wantUntrusted: false,
		},
		{
			name:          "elevated event with base ref is trusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: baseRepo},
			req:           CloneRequest{Ref: "refs/heads/main", URI: "https://github.com/acme/infra.git"},
			wantUntrusted: false,
		},
		{
			name:          "elevated event with same-repo URI (with .git, mixed case) is trusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: "Acme/Infra"},
			req:           CloneRequest{URI: "https://github.com/acme/infra.git"},
			wantUntrusted: false,
		},
		{
			name:          "elevated event with PR merge ref override is untrusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: baseRepo},
			req:           CloneRequest{Ref: "refs/pull/42/merge", URI: "https://github.com/acme/infra.git"},
			wantUntrusted: true,
		},
		{
			name:          "elevated event with PR head ref override is untrusted",
			ctx:           &Context{EventName: "workflow_run", ElevatedEvent: true, Repository: baseRepo},
			req:           CloneRequest{Ref: "refs/pull/7/head"},
			wantUntrusted: true,
		},
		{
			name:          "elevated event with cross-repo https URI is untrusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: baseRepo},
			req:           CloneRequest{URI: "https://github.com/attacker/infra.git"},
			wantUntrusted: true,
		},
		{
			name:          "elevated event with cross-repo scp URI is untrusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: baseRepo},
			req:           CloneRequest{URI: "git@github.com:attacker/infra.git"},
			wantUntrusted: true,
		},
		{
			name:          "elevated event with no ref and no URI is trusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: baseRepo},
			req:           CloneRequest{},
			wantUntrusted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateForkCheckout(tt.ctx, tt.req)
			assert.Equal(t, tt.wantUntrusted, got.Untrusted)
			if tt.wantUntrusted {
				require.NotEmpty(t, got.Reason, "untrusted verdict must carry a reason")
			} else {
				assert.Empty(t, got.Reason)
			}
		})
	}
}

func TestIsPullRequestRefHelper(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"refs/pull/42/merge", true},
		{"refs/pull/42/head", true},
		{"pull/42/merge", true},
		{"refs/pull/1/MERGE", true}, // case-insensitive.
		{" refs/pull/9/merge ", true},
		{"refs/heads/main", false},
		{"main", false},
		{"refs/tags/v1.0.0", false},
		{"refs/pull/42/foo", false}, // not head/merge.
		{"refs/pull/abc/merge", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			assert.Equal(t, tt.want, isPullRequestRef(tt.ref))
		})
	}
}

func TestRepoSlugFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"https://github.com/acme/infra.git", "acme/infra"},
		{"https://github.com/acme/infra", "acme/infra"},
		{"git@github.com:acme/infra.git", "acme/infra"},
		{"ssh://git@github.com/acme/infra.git", "acme/infra"},
		{"git://github.com/acme/infra.git", "acme/infra"},
		{"git::https://github.com/acme/infra.git", "acme/infra"},
		{"ssh://git@github.com:22/acme/infra.git", "acme/infra"}, // port is not a slug.
		{"https://user@example.com/acme/infra.git", "acme/infra"},
		{"https://ghe.example.com/group/sub/infra.git", "sub/infra"}, // last two segments.
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			assert.Equal(t, tt.want, repoSlugFromURI(tt.uri))
		})
	}
}
