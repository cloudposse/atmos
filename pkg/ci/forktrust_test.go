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
			name:          "elevated event with same-slug different-host URI is untrusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: baseRepo, ServerURL: "https://github.com", CloneURL: "https://github.com/acme/infra.git"},
			req:           CloneRequest{URI: "https://evil.example.com/acme/infra.git"},
			wantUntrusted: true,
		},
		{
			name:          "elevated event with matching enterprise host and slug is trusted",
			ctx:           &Context{EventName: "pull_request_target", ElevatedEvent: true, Repository: "acme/infra", ServerURL: "https://ghe.acme.com", CloneURL: "https://ghe.acme.com/acme/infra.git"},
			req:           CloneRequest{URI: "https://ghe.acme.com/acme/infra.git"},
			wantUntrusted: false,
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
		{"https://github.com/justrepo", "justrepo"},                  // single path segment.
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			assert.Equal(t, tt.want, repoSlugFromURI(tt.uri))
		})
	}
}

func TestBaseHostFromContext(t *testing.T) {
	// ServerURL preferred when set.
	assert.Equal(t, "github.com", baseHostFromContext(&Context{ServerURL: "https://github.com", CloneURL: "https://ghe.acme.com/a/b.git"}))
	// Falls back to CloneURL when ServerURL is empty.
	assert.Equal(t, "ghe.acme.com", baseHostFromContext(&Context{ServerURL: "", CloneURL: "https://ghe.acme.com/a/b.git"}))
	// Empty when neither is set.
	assert.Equal(t, "", baseHostFromContext(&Context{}))
}

func TestEvaluateForkCheckout_CloneURLHostFallback(t *testing.T) {
	// ServerURL empty: host comparison must still work via CloneURL.
	ctx := &Context{
		EventName: "pull_request_target", ElevatedEvent: true,
		Repository: "acme/infra", CloneURL: "https://github.com/acme/infra.git",
	}
	got := EvaluateForkCheckout(ctx, CloneRequest{URI: "https://evil.example.com/acme/infra.git"})
	assert.True(t, got.Untrusted)
}

func TestHostFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"https://github.com/acme/infra.git", "github.com"},
		{"https://GitHub.com/acme/infra", "github.com"}, // lowercased.
		{"git@github.com:acme/infra.git", "github.com"},
		{"ssh://git@ghe.acme.com/acme/infra.git", "ghe.acme.com"},
		{"ssh://git@ghe.acme.com:22/acme/infra.git", "ghe.acme.com"}, // port stripped.
		{"git::https://ghe.acme.com/acme/infra.git", "ghe.acme.com"},
		{"https://user@example.com/acme/infra.git", "example.com"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			assert.Equal(t, tt.want, hostFromURI(tt.uri))
		})
	}
}
