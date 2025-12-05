/*
NOTES:

The functions in this file are taken from https://github.com/hashicorp/go-getter/blob/main/get_git.go

We upgrade `go-getter` from v1.7.8 to v1.7.9 because the dependency github.com/hashicorp/go-getter:v1.7.8 was vulnerable:
CVE-2025-8959, Score: 7.5
HashiCorp's go-getter library subdirectory download feature is vulnerable to symlink attacks leading to unauthorized
read access beyond the designated directory boundaries. This vulnerability, identified as CVE-2025-8959,
is fixed in go-getter 1.7.9.
Read More: https://www.mend.io/vulnerability-database/CVE-2025-8959

v1.7.9 completely disabled symlinks when downloading from Git repositories, which broke our code.
In particular, the changes in this function:

func (g *GitGetter) fetchSubmodules(ctx context.Context, dst, sshKeyFile string, depth int) error {
	if g.client != nil {
		g.client.DisableSymlinks = true
	}
}

HashiCorp always sets `DisableSymlinks = true` regardless of the client configuration:

type Client struct {
	// Disable symlinks
	DisableSymlinks bool
}

The `DisableSymlinks` field is configurable for any other protocols (http, https, s3), but not for `git`.

We want it to be configurable because in many cases, a repo has some symlinks, and when `go-getter` downloads modules from
the repo, it downloads the entire repo first into a temp directory, and then gets the requested module from it.
Since `client.DisableSymlinks = true` is always set for the `git` protocol, the code breaks with the error:

ERRO Failed to vendor github/stargazers: copying of symlinks has been disabled

SUMMARY:

- We are using the latest version v1.7.9 of `go-getter`
- We copied the functions from `go-getter` into this file and removed the following code
  (which always disabled symlinks for the `git` protocol):

	if g.client != nil {
	   g.client.DisableSymlinks = true
	}

- This allows us to configure `DisableSymlinks` in our code for the git` protocol
- We'll monitor the progress of https://github.com/hashicorp/go-getter and update our code if HashiCorp makes any changes
  or makes `DisableSymlinks` configurable for the `git` protocol (in which case, we will remove this file and use the native
  `go-getter` functionality)
*/

package downloader

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/hashicorp/go-version"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// Constants for git operations.
const (
	// Numeric constants.
	base10         = 10
	portBitSize    = 16
	sshKeyFileMode = 0o600

	// String constants for git commands.
	gitCommand      = "git"
	originRemote    = "origin"
	gitArgSeparator = "--"
)

// gitOperationParams holds parameters for git operations to reduce function arguments.
type gitOperationParams struct {
	ctx        context.Context
	dst        string
	sshKeyFile string
	u          *url.URL
	ref        string
	depth      int
}

var lsRemoteSymRefRegexp = regexp.MustCompile(`ref: refs/heads/([^\s]+).*`)

// gitCommitIDRegex is a pattern intended to match strings that seem
// "likely to be" git commit IDs, rather than named refs. This cannot be
// an exact decision because it's valid to name a branch or tag after a series
// of hexadecimal digits too.
//
// We require at least 7 digits here because that's the smallest size git
// itself will typically generate, and so it'll reduce the risk of false
// positives on short branch names that happen to also be "hex words".
var gitCommitIDRegex = regexp.MustCompile("^[0-9a-fA-F]{7,40}$")

func (g *CustomGitGetter) GetCustom(dst string, u *url.URL) error {
	ctx := g.Context()

	if g.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.Timeout)
		defer cancel()
	}

	if _, err := exec.LookPath(gitCommand); err != nil {
		return errUtils.ErrGitNotAvailable
	}

	// The port number must be parseable as an integer. If not, the user
	// was probably trying to use a scp-style address, in which case the
	// ssh:// prefix must be removed to indicate that.
	//
	// This is not necessary in versions of Go which have patched
	// CVE-2019-14809 (e.g. Go 1.12.8+)
	if portStr := u.Port(); portStr != "" {
		if _, err := strconv.ParseUint(portStr, base10, portBitSize); err != nil {
			return fmt.Errorf("%w %q; if using the \"scp-like\" git address scheme where a colon introduces the path instead, remove the ssh:// portion and use just the git:: prefix", errUtils.ErrInvalidGitPort, portStr)
		}
	}

	// Extract some query parameters we use
	var ref, sshKey string
	depth := 0 // 0 means "don't use shallow clone"
	q := u.Query()
	if len(q) > 0 {
		ref = q.Get("ref")
		q.Del("ref")

		sshKey = q.Get("sshkey")
		q.Del("sshkey")

		if n, err := strconv.Atoi(q.Get("depth")); err == nil {
			depth = n
		}
		q.Del("depth")

		// Copy the URL
		newU := *u
		u = &newU
		u.RawQuery = q.Encode()
	}

	var sshKeyFile string
	if sshKey != "" {
		// Check that the git version is sufficiently new.
		if err := checkGitVersion(ctx, "2.3"); err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrSSHKeyUsage, err)
		}

		// We have an SSH key - decode it.
		raw, err := base64.StdEncoding.DecodeString(sshKey)
		if err != nil {
			return fmt.Errorf("%w: failed to decode SSH key: %w", errUtils.ErrSSHKeyUsage, err)
		}

		// Create a temp file for the key and ensure it is removed.
		fh, err := os.CreateTemp("", "go-getter")
		if err != nil {
			return fmt.Errorf("%w: failed to create temp file for SSH key: %w", errUtils.ErrSSHKeyUsage, err)
		}
		sshKeyFile = fh.Name()
		defer func() {
			if err := os.Remove(sshKeyFile); err != nil && !os.IsNotExist(err) {
				log.Trace("Failed to remove temporary SSH key file", "error", err, "file", sshKeyFile)
			}
		}()

		// Set the permissions prior to writing the key material.
		if err := os.Chmod(sshKeyFile, sshKeyFileMode); err != nil {
			return fmt.Errorf("%w: failed to set SSH key file permissions: %w", errUtils.ErrSSHKeyUsage, err)
		}

		// Write the raw key into the temp file.
		_, err = fh.Write(raw)
		if closeErr := fh.Close(); closeErr != nil {
			log.Trace("Failed to close temporary SSH key file", "error", closeErr, "file", sshKeyFile)
		}
		if err != nil {
			return fmt.Errorf("%w: failed to write SSH key to temp file: %w", errUtils.ErrSSHKeyUsage, err)
		}
	}

	// Clone or update the repository
	_, err := os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	params := gitOperationParams{
		ctx:        ctx,
		dst:        dst,
		sshKeyFile: sshKeyFile,
		u:          u,
		ref:        ref,
		depth:      depth,
	}

	if err == nil {
		err = g.update(&params)
	} else {
		err = g.clone(&params)
	}
	if err != nil {
		return err
	}

	// Next: check out the proper tag/branch if it is specified, and checkout
	if ref != "" {
		if err := g.checkout(ctx, dst, ref); err != nil {
			return err
		}
	}

	// Lastly, download any/all submodules.
	return g.fetchSubmodules(ctx, dst, sshKeyFile, depth)
}

// setupGitEnv sets up the environment for the given command. This is used to
// pass configuration data to git and ssh and enables advanced cloning methods.
func setupGitEnv(cmd *exec.Cmd, sshKeyFile string) {
	// If there's no sshKeyFile argument to deal with, we can skip this
	// entirely.
	if sshKeyFile == "" {
		return
	}
	const gitSSHCommand = "GIT_SSH_COMMAND="
	var sshCmd []string

	// If we have an existing GIT_SSH_COMMAND, we need to append our options.
	// We will also remove our old entry to make sure the behavior is the same
	// with versions of Go < 1.9.
	env := os.Environ()
	for i, v := range env {
		if strings.HasPrefix(v, gitSSHCommand) && len(v) > len(gitSSHCommand) {
			sshCmd = []string{v}

			env[i], env[len(env)-1] = env[len(env)-1], env[i]
			env = env[:len(env)-1]
			break
		}
	}

	if len(sshCmd) == 0 {
		sshCmd = []string{gitSSHCommand + "ssh"}
	}

	// We have an SSH key temp file configured, tell ssh about this.
	if runtime.GOOS == "windows" {
		sshKeyFile = strings.ReplaceAll(sshKeyFile, `\`, `/`)
	}
	sshCmd = append(sshCmd, "-i", sshKeyFile)
	env = append(env, strings.Join(sshCmd, " "))

	cmd.Env = env
}

// In the case an error happens.
func getRunCommand(cmd *exec.Cmd) error {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err == nil {
		return nil
	}
	exiterr := &exec.ExitError{}
	if errors.As(err, &exiterr) {
		// The program has exited with an exit code != 0
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return fmt.Errorf(
				"%w: %s exited with %d: %s",
				errUtils.ErrGitCommandExited,
				cmd.Path,
				status.ExitStatus(),
				buf.String())
		}
	}

	return fmt.Errorf("%w: %s: %s", errUtils.ErrGitCommandFailed, cmd.Path, buf.String())
}

// removeCaseInsensitiveGitDirectory removes all .git directory variations.
func removeCaseInsensitiveGitDirectory(dst string) error {
	files, err := os.ReadDir(dst)
	if err != nil {
		return fmt.Errorf("%w: %s", errUtils.ErrReadDestDir, dst)
	}
	for _, f := range files {
		if strings.EqualFold(f.Name(), ".git") && f.IsDir() {
			err := os.RemoveAll(filepath.Join(dst, f.Name()))
			if err != nil {
				return fmt.Errorf("%w: %s", errUtils.ErrRemoveGitDir, dst)
			}
		}
	}
	return nil
}

// findRemoteDefaultBranch checks the remote repo's HEAD symref to return the remote repo's default branch. "master" is returned if no HEAD symref exists.
func findRemoteDefaultBranch(ctx context.Context, u *url.URL) string {
	var stdoutbuf bytes.Buffer
	// #nosec G204 -- The URL is validated and we use "--" separator to prevent command injection.
	cmd := exec.CommandContext(ctx, gitCommand, "ls-remote", "--symref", gitArgSeparator, u.String(), "HEAD")
	cmd.Stdout = &stdoutbuf
	err := cmd.Run()
	matches := lsRemoteSymRefRegexp.FindStringSubmatch(stdoutbuf.String())
	if err != nil || matches == nil {
		return "master"
	}
	return matches[len(matches)-1]
}

// checkGitVersion is used to check the version of git installed on the system
// against a known minimum version. Returns an error if the installed version
// is older than the given minimum.
func checkGitVersion(ctx context.Context, min string) error {
	want, err := version.NewVersion(min)
	if err != nil {
		return err
	}

	out, err := exec.CommandContext(ctx, gitCommand, "version").Output()
	if err != nil {
		return err
	}

	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return fmt.Errorf("%w: %q", errUtils.ErrUnexpectedGitOutput, string(out))
	}
	v := fields[2]
	if runtime.GOOS == "windows" && strings.Contains(v, ".windows.") {
		// on windows, git version will return for example:
		// git version 2.20.1.windows.1
		// Which does not follow the semantic versionning specs
		// https://semver.org. We remove that part in order for
		// go-version to not error.
		if idx := strings.Index(v, ".windows."); idx != -1 {
			v = v[:idx]
		}
	}

	have, err := version.NewVersion(v)
	if err != nil {
		return err
	}

	if have.LessThan(want) {
		return fmt.Errorf("%w: required git version = %s, have %s", errUtils.ErrGitVersionMismatch, want, have)
	}

	return nil
}

func (g *CustomGitGetter) checkout(ctx context.Context, dst string, ref string) error {
	cmd := exec.CommandContext(ctx, gitCommand, "checkout", ref)
	cmd.Dir = dst
	return getRunCommand(cmd)
}

func (g *CustomGitGetter) clone(params *gitOperationParams) error {
	ctx := params.ctx
	dst := params.dst
	sshKeyFile := params.sshKeyFile
	u := params.u
	ref := params.ref
	depth := params.depth

	args := []string{"clone"}

	originalRef := ref // we handle an unspecified ref differently than explicitly selecting the default branch below
	if ref == "" {
		ref = findRemoteDefaultBranch(ctx, u)
	}
	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth))
		args = append(args, "--branch", ref)
	}
	args = append(args, gitArgSeparator, u.String(), dst)

	cmd := exec.CommandContext(ctx, gitCommand, args...)
	setupGitEnv(cmd, sshKeyFile)
	err := getRunCommand(cmd)
	if err != nil {
		if depth > 0 && originalRef != "" {
			// If we're creating a shallow clone then the given ref must be
			// a named ref (branch or tag) rather than a commit directly.
			// We can't accurately recognize the resulting error here without
			// hard-coding assumptions about git's human-readable output, but
			// we can at least try a heuristic.
			if gitCommitIDRegex.MatchString(originalRef) {
				return fmt.Errorf("%w (note that setting 'depth' requires 'ref' to be a branch or tag name)", err)
			}
		}
		return err
	}

	if depth < 1 && originalRef != "" {
		// If we didn't add --depth and --branch above then we will now be
		// on the remote repository's default branch, rather than the selected
		// ref, so we'll need to fix that before we return.
		err := g.checkout(ctx, dst, originalRef)
		if err != nil {
			// Clean up git repository on disk
			if removeErr := os.RemoveAll(dst); removeErr != nil {
				log.Trace("Failed to remove git repository during cleanup", "error", removeErr, "dir", dst)
			}
			return err
		}
	}
	return nil
}

func (g *CustomGitGetter) update(params *gitOperationParams) error {
	ctx := params.ctx
	dst := params.dst
	sshKeyFile := params.sshKeyFile
	u := params.u
	ref := params.ref
	depth := params.depth

	// Remove all variations of .git directories
	err := removeCaseInsensitiveGitDirectory(dst)
	if err != nil {
		return err
	}

	// Initialize the git repository
	cmd := exec.CommandContext(ctx, gitCommand, "init")
	cmd.Dir = dst
	err = getRunCommand(cmd)
	if err != nil {
		return err
	}

	// Add the git remote
	// #nosec G204 -- The URL is validated and we use "--" separator to prevent command injection.
	cmd = exec.CommandContext(ctx, gitCommand, "remote", "add", originRemote, gitArgSeparator, u.String())
	cmd.Dir = dst
	err = getRunCommand(cmd)
	if err != nil {
		return err
	}

	// Fetch the remote ref
	cmd = exec.CommandContext(ctx, gitCommand, "fetch", "--tags")
	cmd.Dir = dst
	err = getRunCommand(cmd)
	if err != nil {
		return err
	}

	// Fetch the remote ref
	cmd = exec.CommandContext(ctx, gitCommand, "fetch", originRemote, gitArgSeparator, ref)
	cmd.Dir = dst
	err = getRunCommand(cmd)
	if err != nil {
		return err
	}

	// Reset the branch to the fetched ref
	cmd = exec.CommandContext(ctx, gitCommand, "reset", "--hard", "FETCH_HEAD")
	cmd.Dir = dst
	err = getRunCommand(cmd)
	if err != nil {
		return err
	}

	// Checkout ref branch
	err = g.checkout(ctx, dst, ref)
	if err != nil {
		return err
	}

	// Pull the latest changes from the ref branch
	if depth > 0 {
		// #nosec G204 -- The ref is from query parameters and we use "--" separator to prevent command injection.
		cmd = exec.CommandContext(ctx, gitCommand, "pull", originRemote, "--depth", strconv.Itoa(depth), "--ff-only", gitArgSeparator, ref)
	} else {
		cmd = exec.CommandContext(ctx, gitCommand, "pull", originRemote, "--ff-only", gitArgSeparator, ref)
	}

	cmd.Dir = dst
	setupGitEnv(cmd, sshKeyFile)
	return getRunCommand(cmd)
}

// fetchSubmodules downloads any configured submodules recursively.
func (g *CustomGitGetter) fetchSubmodules(ctx context.Context, dst, sshKeyFile string, depth int) error {
	args := []string{"submodule", "update", "--init", "--recursive"}
	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth))
	}
	cmd := exec.CommandContext(ctx, gitCommand, args...)
	cmd.Dir = dst
	setupGitEnv(cmd, sshKeyFile)
	return getRunCommand(cmd)
}
