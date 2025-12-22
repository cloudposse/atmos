package exec

import (
	"errors"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	errUtils "github.com/cloudposse/atmos/errors"
	g "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var RemoteRepoIsNotGitRepoError = errors.New("the target remote repo is not a Git repository. Check that it was initialized and has '.git' folder")

const (
	shaString = "SHA"
	refString = "ref"
)

// ExecuteDescribeAffectedWithTargetRefClone clones the remote reference,
// processes stack configs, and returns a list of the affected Atmos components and stacks given two Git commits.
func ExecuteDescribeAffectedWithTargetRefClone(
	atmosConfig *schema.AtmosConfiguration,
	ref string,
	sha string,
	sshKeyPath string,
	sshKeyPassword string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	excludeLocked bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeAffectedWithTargetRefClone")()

	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Clone the remote repo
	// https://git-scm.com/book/en/v2/Git-Internals-Git-References
	// https://git-scm.com/docs/git-show-ref
	// https://github.com/go-git/go-git/tree/master/_examples
	// https://stackoverflow.com/questions/56810719/how-to-checkout-a-specific-sha-in-a-git-repo-using-golang
	// https://golang.hotexamples.com/examples/gopkg.in.src-d.go-git.v4.plumbing/-/ReferenceName/golang-referencename-function-examples.html
	// https://stackoverflow.com/questions/58905690/how-to-identify-which-files-have-changed-between-git-commits
	// https://github.com/src-d/go-git/issues/604

	// Create a temp dir to clone the remote repo to
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, nil, nil, "", err
	}

	log.Debug("Cloning repo into temp directory", "repo", localRepoInfo.RepoUrl, "dir", tempDir)

	cloneOptions := git.CloneOptions{
		URL:          localRepoInfo.RepoUrl,
		NoCheckout:   false,
		SingleBranch: false,
	}

	// If `ref` flag is not provided, it will clone the HEAD of the default branch
	if ref != "" {
		cloneOptions.ReferenceName = plumbing.ReferenceName(ref)
		log.Debug("Cloning Git", refString, ref)
	} else {
		log.Debug("Cloned the HEAD of the default branch")
	}

	if atmosConfig.Logs.Level == u.LogLevelDebug {
		cloneOptions.Progress = os.Stdout
	}

	// Clone private repos using SSH
	// https://gist.github.com/efontan/e8e8818dc0845d3bd7bf1343c984ae7b
	// https://github.com/src-d/go-git/issues/550
	if sshKeyPath != "" {
		sshKeyContent, err := os.ReadFile(sshKeyPath)
		if err != nil {
			return nil, nil, nil, "", err
		}

		sshPublicKey, err := ssh.NewPublicKeys("git", sshKeyContent, sshKeyPassword)
		if err != nil {
			return nil, nil, nil, "", err
		}

		// Use the SSH key to clone the repo
		cloneOptions.Auth = sshPublicKey

		// Update the repo URL to SSH format
		// https://mirrors.edge.kernel.org/pub/software/scm/git/docs/git-clone.html
		cloneOptions.URL = strings.Replace(localRepoInfo.RepoUrl, "https://", "ssh://", 1)
	}

	remoteRepo, err := git.PlainClone(tempDir, false, &cloneOptions)
	if err != nil {
		return nil, nil, nil, "", err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, nil, nil, "", err
	}

	if ref != "" {
		log.Debug("Cloned Git", refString, ref)
	} else {
		log.Debug("Cloned Git", refString, remoteRepoHead.Name())
	}

	// Check if a commit SHA was provided and check out the repo at that commit SHA
	if sha != "" {
		log.Debug("Checking out commit", shaString, sha)

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, nil, nil, "", err
		}

		checkoutOptions := git.CheckoutOptions{
			Hash:   plumbing.NewHash(sha),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			return nil, nil, nil, "", err
		}

		log.Debug("Checked out commit", shaString, sha)
	}

	affected, localRepoHead, remoteRepoHead, err := executeDescribeAffected(
		atmosConfig,
		localRepoInfo.LocalWorktreePath,
		tempDir,
		localRepo,
		remoteRepo,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		processTemplates,
		processYamlFunctions,
		skip,
		excludeLocked,
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	/*
		Do not use `defer removeTempDir(tempDir)` right after the temp dir is created, instead call `removeTempDir(tempDir)` at the end of the main function:
		 - On Windows, there are race conditions when using `defer` and goroutines
		 - We defer removeTempDir(tempDir) right after creating the temp dir
		 - We `git clone` a repo into it
		 - We then start goroutines that read files from the temp dir
		 - Meanwhile, when the main function exits, defer removeTempDir(...) runs
		 - On Windows, open file handles in goroutines make directory deletion flaky or fail entirely (and possibly prematurely delete files while goroutines are mid-read)
	*/
	removeTempDir(tempDir)

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}

// ExecuteDescribeAffectedWithTargetRefCheckout checks out the target reference using git worktree,
// processes stack configs, and returns a list of the affected Atmos components and stacks given two Git commits.
// This approach uses `git worktree add` to create an isolated worktree that shares the repository's
// object database but has its own HEAD, allowing checkout operations without affecting the main worktree.
func ExecuteDescribeAffectedWithTargetRefCheckout(
	atmosConfig *schema.AtmosConfiguration,
	ref string,
	sha string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	excludeLocked bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeAffectedWithTargetRefCheckout")()

	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Determine the target commit for the worktree.
	var targetCommit string
	if sha != "" {
		targetCommit = sha
		log.Debug("Creating worktree at commit", shaString, sha)
	} else {
		// If `ref` is not provided, use the HEAD of the remote origin.
		if ref == "" {
			ref = "refs/remotes/origin/HEAD"
		}
		targetCommit = ref
		log.Debug("Creating worktree at", refString, ref)
	}

	// Create an isolated worktree for the target ref.
	worktreePath, err := g.CreateWorktree(localRepoInfo.LocalWorktreePath, targetCommit)
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Deferred cleanup ensures worktree is always removed.
	defer cleanupWorktree(localRepoInfo.LocalWorktreePath, worktreePath)

	// Open the worktree as a git repository.
	remoteRepo, err := g.OpenWorktreeAwareRepo(worktreePath)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, RemoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the target ref.
	_, err = g.GetRepoConfig(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, RemoteRepoIsNotGitRepoError)
	}

	affected, localRepoHead, remoteRepoHead, err := executeDescribeAffected(
		atmosConfig,
		localRepoInfo.LocalWorktreePath,
		worktreePath,
		localRepo,
		remoteRepo,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		processTemplates,
		processYamlFunctions,
		skip,
		excludeLocked,
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}

// cleanupWorktree removes a git worktree and its parent temp directory.
func cleanupWorktree(repoPath, worktreePath string) {
	g.RemoveWorktree(repoPath, worktreePath)
	removeTempDir(g.GetWorktreeParentDir(worktreePath))
}

// ExecuteDescribeAffectedWithTargetRepoPath uses `repo-path` to access the target repo, and processes stack configs
// and returns a list of the affected Atmos components and stacks given two Git commits.
func ExecuteDescribeAffectedWithTargetRepoPath(
	atmosConfig *schema.AtmosConfiguration,
	targetRefPath string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	excludeLocked bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeAffectedWithTargetRepoPath")()

	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Use worktree-aware helper to open the repository at the target path
	// This handles both regular repositories and worktrees correctly
	remoteRepo, err := g.OpenWorktreeAwareRepo(targetRefPath)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, RemoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the remote target repo
	_, err = g.GetRepoConfig(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, RemoteRepoIsNotGitRepoError)
	}

	remoteRepoInfo, err := g.GetRepoInfo(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	affected, localRepoHead, remoteRepoHead, err := executeDescribeAffected(
		atmosConfig,
		localRepoInfo.LocalWorktreePath,
		remoteRepoInfo.LocalWorktreePath,
		localRepo,
		remoteRepo,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		processTemplates,
		processYamlFunctions,
		skip,
		excludeLocked,
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}
