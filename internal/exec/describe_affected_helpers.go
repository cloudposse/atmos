package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	cp "github.com/otiai10/copy"

	g "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var remoteRepoIsNotGitRepoError = errors.New("the target remote repo is not a Git repository. Check that it was initialized and has '.git' folder")

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
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
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

// ExecuteDescribeAffectedWithTargetRefCheckout checks out the target reference,
// processes stack configs, and returns a list of the affected Atmos components and stacks given two Git commits.
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
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Create a temp dir for the target ref
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Copy the local repo into the temp directory
	log.Debug("Copying the local repo into temp directory", "dir", tempDir)

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		// Skip specifies which files should be skipped
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") {
				return true, nil
			}

			// Check if the file is a socket and skip it
			isSocket, err := u.IsSocket(src)
			if err != nil {
				return true, err
			}
			if isSocket {
				return true, nil
			}

			return false, nil
		},
	}

	if err = cp.Copy(localRepoInfo.LocalWorktreePath, tempDir, copyOptions); err != nil {
		return nil, nil, nil, "", err
	}

	log.Debug("Copied the local repo into temp directory", "dir", tempDir)

	remoteRepo, err := git.PlainOpenWithOptions(tempDir, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the target ref
	_, err = g.GetRepoConfig(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

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
	} else {
		// If `ref` is not provided, use the HEAD of the remote origin
		if ref == "" {
			ref = "refs/remotes/origin/HEAD"
		}

		log.Debug("Checking out Git", refString, ref)

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, nil, nil, "", err
		}

		checkoutOptions := git.CheckoutOptions{
			Branch: plumbing.ReferenceName(ref),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			if strings.Contains(err.Error(), "reference not found") {
				errorMessage := fmt.Sprintf("the Git ref '%s' does not exist on the local filesystem"+
					"\nmake sure it's correct and was cloned by Git from the remote, or use the '--clone-target-ref=true' flag to clone it"+
					"\nrefer to https://atmos.tools/cli/commands/describe/affected for more details", ref)
				err = errors.New(errorMessage)
			}
			return nil, nil, nil, "", err
		}

		log.Debug("Checked out Git", refString, ref)
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
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	remoteRepo, err := git.PlainOpenWithOptions(targetRefPath, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the remote target repo
	_, err = g.GetRepoConfig(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
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
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}
