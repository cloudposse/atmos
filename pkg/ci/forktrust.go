package ci

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// gitGetterPrefix is the go-getter forcing prefix stripped before slug parsing.
const gitGetterPrefix = "git::"

// pullRequestRefPattern matches refs that resolve to pull-request head or merge
// content (e.g. "refs/pull/42/merge", "refs/pull/42/head", "pull/42/merge").
// These are the refs an actions/checkout-style "pwn request" uses to pull a
// fork's PR code, so requesting one under an elevated event is gated.
var pullRequestRefPattern = regexp.MustCompile(`(?i)(^|/)pull/\d+/(merge|head)$`)

// CloneRequest describes a requested git clone for fork-checkout trust
// evaluation. It carries only what the gate needs: the ref/branch that will be
// checked out and the clone target URI (used to detect cross-repository clones).
type CloneRequest struct {
	// Ref is the resolved ref/branch to be checked out (may be empty).
	Ref string

	// URI is the clone target URI (may be empty for named/no-arg clones whose
	// target is the CI repository itself).
	URI string
}

// ForkVerdict is the result of EvaluateForkCheckout.
type ForkVerdict struct {
	// Untrusted reports whether the requested clone would fetch untrusted fork
	// content under an elevated CI event and therefore requires an opt-in.
	Untrusted bool

	// Reason is a human-readable explanation of why the clone was flagged,
	// suitable for an error explanation. Empty when Untrusted is false.
	Reason string
}

// EvaluateForkCheckout reports whether cloning req under the current CI event
// would fetch untrusted fork content. It is the provider-agnostic core of the
// fork-checkout safety gate that mirrors actions/checkout v7's refusal to fetch
// fork PR code in pull_request_target / workflow_run workflows.
//
// A clone is flagged only when BOTH hold:
//  1. the event is elevated (ciCtx.ElevatedEvent — set by the provider), and
//  2. the request targets fork content: an explicit ref override that is a PR
//     head/merge ref, or a clone URI whose owner/repo differs from the base
//     CI repository.
//
// The safe no-arg default (clone the base repository at its base ref) is never
// flagged. See docs/prd/native-ci/framework/fork-pr-trust-gate.md.
func EvaluateForkCheckout(ciCtx *Context, req CloneRequest) ForkVerdict {
	defer perf.Track(nil, "ci.EvaluateForkCheckout")()

	if ciCtx == nil || !ciCtx.ElevatedEvent {
		return ForkVerdict{}
	}

	if ref := strings.TrimSpace(req.Ref); ref != "" && isPullRequestRef(ref) {
		return ForkVerdict{
			Untrusted: true,
			Reason:    fmt.Sprintf("requested ref %q resolves to pull-request head/merge content from a fork", ref),
		}
	}

	return evaluateCloneTarget(ciCtx, req.URI)
}

// evaluateCloneTarget reports whether a clone URI points at a different remote
// than the base CI repository — by host or by owner/repo. A different host is
// untrusted even when the owner/repo slug matches, so
// "https://evil.example.com/acme/infra.git" cannot pass as the base "acme/infra".
func evaluateCloneTarget(ciCtx *Context, uri string) ForkVerdict {
	if uri == "" || ciCtx.Repository == "" {
		return ForkVerdict{}
	}

	baseHost := baseHostFromContext(ciCtx)
	targetHost := hostFromURI(uri)
	if baseHost != "" && targetHost != "" && !strings.EqualFold(targetHost, baseHost) {
		return ForkVerdict{
			Untrusted: true,
			Reason:    fmt.Sprintf("clone target host %q differs from the base repository host %q", targetHost, baseHost),
		}
	}

	targetSlug := repoSlugFromURI(uri)
	baseSlug := normalizeRepoSlug(ciCtx.Repository)
	if targetSlug != "" && baseSlug != "" && targetSlug != baseSlug {
		return ForkVerdict{
			Untrusted: true,
			Reason:    fmt.Sprintf("clone target %q differs from the base repository %q", targetSlug, baseSlug),
		}
	}

	return ForkVerdict{}
}

// isPullRequestRef reports whether ref denotes pull-request head or merge
// content (e.g. "refs/pull/42/merge").
func isPullRequestRef(ref string) bool {
	return pullRequestRefPattern.MatchString(strings.TrimSpace(ref))
}

// repoSlugFromURI extracts a normalized "owner/repo" slug from a git clone URI,
// handling go-getter, https/ssh/git, and SCP-style (git@host:owner/repo) forms.
// Returns an empty string when a slug cannot be derived.
func repoSlugFromURI(raw string) string {
	stripped := strings.TrimPrefix(strings.TrimSpace(raw), gitGetterPrefix)
	if stripped == "" {
		return ""
	}

	// SCP-style: git@host:owner/repo(.git) — net/url cannot parse this form.
	if at := strings.Index(stripped, "@"); at >= 0 {
		if colon := strings.Index(stripped, ":"); colon > at {
			afterColon := stripped[colon+1:]
			// Only treat as SCP-style when the segment after ":" is not a port
			// (i.e. it is not purely numeric before the first slash).
			if !strings.HasPrefix(afterColon, "//") {
				return normalizeRepoSlug(afterColon)
			}
		}
	}

	u, err := url.Parse(stripped)
	if err != nil {
		return ""
	}
	return normalizeRepoSlug(u.Path)
}

// baseHostFromContext returns the lowercased host of the base CI repository,
// preferring ServerURL and falling back to CloneURL. Empty when neither is set.
func baseHostFromContext(ciCtx *Context) string {
	if h := hostFromURI(ciCtx.ServerURL); h != "" {
		return h
	}
	return hostFromURI(ciCtx.CloneURL)
}

// hostFromURI extracts the lowercased host from a git clone URI, handling
// go-getter, https/ssh/git, and SCP-style (git@host:owner/repo) forms. Returns
// an empty string when a host cannot be derived.
func hostFromURI(raw string) string {
	stripped := strings.TrimPrefix(strings.TrimSpace(raw), gitGetterPrefix)
	if stripped == "" {
		return ""
	}

	// SCP-style: git@host:owner/repo(.git) — net/url cannot parse this form.
	if at := strings.Index(stripped, "@"); at >= 0 {
		if colon := strings.Index(stripped, ":"); colon > at && !strings.HasPrefix(stripped[colon+1:], "//") {
			return strings.ToLower(stripped[at+1 : colon])
		}
	}

	u, err := url.Parse(stripped)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// normalizeRepoSlug reduces a path or "owner/repo" string to a lowercased
// "owner/repo" slug (last two path segments, ".git" suffix removed).
func normalizeRepoSlug(p string) string {
	cleaned := strings.Trim(strings.TrimSpace(p), "/")
	cleaned = strings.TrimSuffix(cleaned, ".git")
	if cleaned == "" {
		return ""
	}

	segments := strings.Split(cleaned, "/")
	if len(segments) >= 2 {
		segments = segments[len(segments)-2:]
	}
	repo := strings.TrimSuffix(path.Base(cleaned), ".git")
	if len(segments) == 2 {
		return strings.ToLower(segments[0] + "/" + repo)
	}
	return strings.ToLower(repo)
}
