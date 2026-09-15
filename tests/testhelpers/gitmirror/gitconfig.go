package gitmirror

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// defaultGitHubUsername is the username atmos's CustomGitDetector pairs with an injected GitHub
// token (see pkg/downloader/custom_git_detector.go's getDefaultUsername), so a rule keyed on it
// matches the exact URL form atmos hands git after injection.
const defaultGitHubUsername = "x-access-token"

// WriteGitConfig writes a git config file at path with insteadOf rules that redirect every
// github.com/<Owner>/<Repo>.git fetch to serverURL (a Server's URL, see Serve): one rule per
// token in tokens, spelled out in the userinfo form atmos hands git after CustomGitDetector
// injects that token, plus one anonymous rule (covering both the https and ssh forms) for
// callers that never inject a token at all.
//
// A separate rule per token is required because git's insteadOf is a plain string-prefix
// rewrite, not URL-aware: "https://x-access-token:TOKEN_A@github.com/..." and
// "https://x-access-token:TOKEN_B@github.com/..." are different strings even though they name
// the same repository, so each token needs its own rule spelling out that exact prefix.
//
// The file is meant to be installed process-wide via the GIT_CONFIG_GLOBAL environment variable
// (not GIT_CONFIG_COUNT/KEY_n/VALUE_n): pkg/downloader/custom_git_detector.go's
// brokerInsteadOfMatchesURL only recognizes a live auth broker's GIT_CONFIG_* env entries, so a
// GIT_CONFIG_GLOBAL file mirror never trips that check and token injection always runs -- the
// same code path a production clone takes.
func WriteGitConfig(path, serverURL string, tokens []string) error {
	mirrorBase, err := url.Parse(strings.TrimSuffix(serverURL, "/") + "/" + Owner + "/" + Repo + ".git")
	if err != nil {
		return fmt.Errorf("gitmirror: parse mirror server URL %q: %w", serverURL, err)
	}

	var b strings.Builder
	for _, token := range tokens {
		authedMirror := *mirrorBase
		authedMirror.User = url.UserPassword(defaultGitHubUsername, token)
		fmt.Fprintf(&b, "[url %q]\n\tinsteadOf = %s\n", authedMirror.String(), githubHTTPSURL(defaultGitHubUsername, token))
	}

	fmt.Fprintf(&b, "[url %q]\n\tinsteadOf = %s\n\tinsteadOf = %s\n", mirrorBase.String(), githubHTTPSURL("", ""), githubSSHURL())

	if err := os.WriteFile(path, []byte(b.String()), filePerm); err != nil {
		return fmt.Errorf("gitmirror: write git config %s: %w", path, err)
	}
	return nil
}

// githubHTTPSURL builds the canonical https://github.com/<Owner>/<Repo>.git URL, optionally
// carrying Basic-Auth userinfo when user is non-empty. It uses the same construction
// CustomGitDetector's injectToken uses (url.UserPassword + URL.String()), so the rendered string
// matches byte-for-byte whatever token injection produces.
func githubHTTPSURL(user, token string) string {
	u := &url.URL{Scheme: "https", Host: "github.com", Path: "/" + Owner + "/" + Repo + ".git"}
	if user != "" {
		u.User = url.UserPassword(user, token)
	}
	return u.String()
}

// githubSSHURL builds the canonical ssh://git@github.com/<Owner>/<Repo>.git URL go-getter hands
// git for an SSH-form source. SSH auth is key-based, not token-based, so this form never carries
// injected userinfo beyond the fixed "git@" convention. Built as a plain string rather than via
// net/url: url.URL treats "git@github.com" as an opaque Host value and percent-encodes the "@",
// which would no longer match the literal string go-getter/git actually produce.
func githubSSHURL() string {
	return "ssh://git@github.com/" + Owner + "/" + Repo + ".git"
}
