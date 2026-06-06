package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v59/github"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/oauth2"
)

// githubTokenEnvVars are the environment variables consulted (in order) for a GitHub token when
// options.token is not set. This mirrors the Atmos precedence in pkg/http; the env cascade is
// inlined here because pkg/store cannot import pkg/http (it transitively imports pkg/store).
var githubTokenEnvVars = []string{"ATMOS_PRO_GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"}

// resolveGitHubToken returns the first non-empty token from the explicit option or the standard
// GitHub token environment variables.
func resolveGitHubToken(explicit string) string {
	if t := strings.TrimSpace(explicit); t != "" {
		return t
	}
	for _, name := range githubTokenEnvVars {
		// These are external GitHub credentials, not Atmos configuration.
		//nolint:forbidigo // GitHub token env vars are external credentials, not Atmos config.
		if t := strings.TrimSpace(os.Getenv(name)); t != "" {
			return t
		}
	}
	return ""
}

// githubAPITimeout bounds GitHub API requests to prevent hangs in CI when the network or DNS is
// unavailable (mirrors the timeout used by pkg/github).
const githubAPITimeout = 30 * time.Second

// githubPublicKeySize is the byte length of a NaCl/libsodium public key (Curve25519).
const githubPublicKeySize = 32

// gitHubActionsClient abstracts the GitHub Actions secrets operations the store needs against a
// single repo (and optional environment) scope. The real implementation wraps go-github; tests
// inject an in-memory fake. Reading a secret's *value* is intentionally absent: the GitHub API
// never returns secret values (they are only injected into a runner's environment), so value
// reads are handled by the store via the process environment, not this client.
type gitHubActionsClient interface {
	// PutSecret encrypts value with the scope's public key (sealed box) and creates/updates the
	// secret named name.
	PutSecret(ctx context.Context, name, value string) error
	// HasSecret reports whether a secret named name exists in the scope (metadata only).
	HasSecret(ctx context.Context, name string) (bool, error)
	// DeleteSecret removes the secret named name. It is idempotent: a missing secret is not an error.
	DeleteSecret(ctx context.Context, name string) error
}

// githubActionsAPIClient is the go-github backed gitHubActionsClient. When environment is empty it
// targets repository secrets; otherwise it targets environment secrets (which the v59 API keys by
// numeric repository ID, resolved lazily on first use).
type githubActionsAPIClient struct {
	gh          *github.Client
	owner       string
	repo        string
	environment string

	idOnce sync.Once
	repoID int
	idErr  error
}

// newGitHubActionsAPIClient builds a go-github client using an explicit token (options.Token) or
// the standard Atmos GitHub token resolution chain (--github-token → ATMOS_GITHUB_TOKEN →
// GITHUB_TOKEN → `gh auth token`).
func newGitHubActionsAPIClient(options *GitHubActionsStoreOptions) (gitHubActionsClient, error) {
	token := resolveGitHubToken(options.Token)

	baseClient := &http.Client{Timeout: githubAPITimeout}
	var gh *github.Client
	if token == "" {
		gh = github.NewClient(baseClient)
	} else {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		tc.Timeout = githubAPITimeout
		gh = github.NewClient(tc)
	}

	return &githubActionsAPIClient{
		gh:          gh,
		owner:       options.Owner,
		repo:        options.Repo,
		environment: options.Environment,
	}, nil
}

// repoIDFor resolves the numeric repository ID required by the environment-secret endpoints.
func (c *githubActionsAPIClient) repoIDFor(ctx context.Context) (int, error) {
	c.idOnce.Do(func() {
		repo, _, err := c.gh.Repositories.Get(ctx, c.owner, c.repo)
		if err != nil {
			c.idErr = fmt.Errorf(errWrapFormat, ErrGitHubResolveRepoID, err)
			return
		}
		c.repoID = int(repo.GetID())
	})
	return c.repoID, c.idErr
}

// publicKey fetches the scope's secret-encryption public key.
func (c *githubActionsAPIClient) publicKey(ctx context.Context) (*github.PublicKey, error) {
	var (
		pk  *github.PublicKey
		err error
	)
	if c.environment == "" {
		pk, _, err = c.gh.Actions.GetRepoPublicKey(ctx, c.owner, c.repo)
	} else {
		repoID, idErr := c.repoIDFor(ctx)
		if idErr != nil {
			return nil, idErr
		}
		pk, _, err = c.gh.Actions.GetEnvPublicKey(ctx, repoID, c.environment)
	}
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGitHubGetPublicKey, err)
	}
	return pk, nil
}

func (c *githubActionsAPIClient) PutSecret(ctx context.Context, name, value string) error {
	pk, err := c.publicKey(ctx)
	if err != nil {
		return err
	}
	encrypted, err := sealSecret(pk.GetKey(), value)
	if err != nil {
		return err
	}
	eSecret := &github.EncryptedSecret{
		Name:           name,
		KeyID:          pk.GetKeyID(),
		EncryptedValue: encrypted,
	}

	if c.environment == "" {
		_, err = c.gh.Actions.CreateOrUpdateRepoSecret(ctx, c.owner, c.repo, eSecret)
	} else {
		repoID, idErr := c.repoIDFor(ctx)
		if idErr != nil {
			return idErr
		}
		_, err = c.gh.Actions.CreateOrUpdateEnvSecret(ctx, repoID, c.environment, eSecret)
	}
	if err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrGitHubPutSecret, name, err)
	}
	return nil
}

func (c *githubActionsAPIClient) HasSecret(ctx context.Context, name string) (bool, error) {
	var (
		resp *github.Response
		err  error
	)
	if c.environment == "" {
		_, resp, err = c.gh.Actions.GetRepoSecret(ctx, c.owner, c.repo, name)
	} else {
		repoID, idErr := c.repoIDFor(ctx)
		if idErr != nil {
			return false, idErr
		}
		_, resp, err = c.gh.Actions.GetEnvSecret(ctx, repoID, c.environment, name)
	}
	if err != nil {
		if ghIsNotFound(resp, err) {
			return false, nil
		}
		return false, fmt.Errorf(errWrapFormatWithID, ErrGitHubGetSecret, name, err)
	}
	return true, nil
}

func (c *githubActionsAPIClient) DeleteSecret(ctx context.Context, name string) error {
	var (
		resp *github.Response
		err  error
	)
	if c.environment == "" {
		resp, err = c.gh.Actions.DeleteRepoSecret(ctx, c.owner, c.repo, name)
	} else {
		repoID, idErr := c.repoIDFor(ctx)
		if idErr != nil {
			return idErr
		}
		resp, err = c.gh.Actions.DeleteEnvSecret(ctx, repoID, c.environment, name)
	}
	if err != nil {
		if ghIsNotFound(resp, err) {
			return nil // Idempotent: nothing to delete.
		}
		return fmt.Errorf(errWrapFormatWithID, ErrGitHubDeleteSecret, name, err)
	}
	return nil
}

// ghIsNotFound reports whether a GitHub API error/response indicates a missing resource (HTTP 404).
func ghIsNotFound(resp *github.Response, err error) bool {
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return true
	}
	var ge *github.ErrorResponse
	if errors.As(err, &ge) && ge.Response != nil {
		return ge.Response.StatusCode == http.StatusNotFound
	}
	return false
}

// sealSecret encrypts plaintext for a GitHub Actions secret using the repo/environment public key.
// GitHub requires libsodium "sealed box" (anonymous) encryption; nacl/box.SealAnonymous is the
// pure-Go equivalent. publicKeyB64 is the base64-encoded Curve25519 public key returned by the
// public-key endpoint; the result is the base64-encoded ciphertext expected by EncryptedValue.
func sealSecret(publicKeyB64, plaintext string) (string, error) {
	pkBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return "", fmt.Errorf(errWrapFormat, ErrGitHubSealSecret, err)
	}
	if len(pkBytes) != githubPublicKeySize {
		return "", fmt.Errorf("%w: got %d bytes, want %d", ErrGitHubPublicKeySize, len(pkBytes), githubPublicKeySize)
	}

	var publicKey [githubPublicKeySize]byte
	copy(publicKey[:], pkBytes)

	sealed, err := box.SealAnonymous(nil, []byte(plaintext), &publicKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf(errWrapFormat, ErrGitHubSealSecret, err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}
