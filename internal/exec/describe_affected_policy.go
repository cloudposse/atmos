package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Affected policies run once for the whole comparison, before any display or
// scheduler filtering. No lifecycle hooks are dispatched by this gate.
func enforceAffectedPolicies(config *schema.AtmosConfiguration, local, remote *git.Repository, localPath, remotePath string) error {
	bundle, err := admission.LoadBundle()
	if err != nil || bundle == nil {
		return err
	}
	rules := bundle.RulesFor(policy.AtmosAffected)
	if len(rules) == 0 {
		return nil
	}
	request, err := collectAffectedPolicyRequest(config, local, remote, localPath, remotePath)
	if err != nil {
		return fmt.Errorf("%w: affected inventory: %w", errUtils.ErrPolicyInputUnavailable, err)
	}
	request.CI, err = ci.CollectPolicySnapshot(context.Background())
	if err != nil {
		return err
	}
	return evaluateAffectedPolicies(request, rules)
}

func evaluateAffectedPolicies(request *policy.Request, rules []policy.Rule) error {
	engine := policy.NewEngine()
	decision, err := engine.Evaluate(context.Background(), request, rules)
	if err != nil {
		// Never print inventory values, which may include sensitive configuration.
		return fmt.Errorf("%w: affected evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	decision, err = ci.ResolvePolicyApprovals(context.Background(), request, decision)
	if err != nil {
		return err
	}
	return policy.EnforceDecision(decision)
}

func collectAffectedPolicyRequest(config *schema.AtmosConfiguration, local, remote *git.Repository, localPath, remotePath string) (*policy.Request, error) {
	for _, path := range []*string{&localPath, &remotePath} {
		absolute, err := filepath.Abs(*path)
		if err != nil {
			return nil, err
		}
		*path = absolute
	}
	return collectAffectedPolicyRequestAtPaths(config, local, remote, localPath, remotePath)
}

func collectAffectedPolicyRequestAtPaths(config *schema.AtmosConfiguration, local, remote *git.Repository, localPath, remotePath string) (*policy.Request, error) {
	afterTree, head, err := affectedPolicyTree(local)
	if err != nil {
		return nil, err
	}
	beforeTree, base, err := affectedPolicyTree(remote)
	if err != nil {
		return nil, err
	}
	files, err := affectedPolicyFiles(beforeTree, afterTree)
	if err != nil {
		return nil, err
	}
	after, err := collectAffectedPolicyInventory(config, localPath, localPath)
	if err != nil {
		return nil, err
	}
	before, err := collectAffectedPolicyInventory(config, localPath, remotePath)
	if err != nil {
		return nil, err
	}
	forced, err := affectedPolicySourceChanges(config, localPath, before, after, files)
	if err != nil {
		return nil, err
	}
	input, err := policy.BuildAffectedInput(before, after, files, forced)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("%w: encode affected inventory", errUtils.ErrPolicyInputInvalid)
	}
	if err := verifyAffectedPolicyRevisions(local, remote, head, base); err != nil {
		return nil, err
	}
	return &policy.Request{Kind: policy.AtmosAffected, Input: data, Revisions: policy.Revisions{Base: base, Head: head}}, nil
}

func affectedPolicyTree(repo *git.Repository) (*object.Tree, string, error) {
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, "", err
	}
	// go-git does not read core.excludesFile, so a file ignored only by the user's global excludes
	// would look untracked and block the analysis. An unreadable global file keeps the strict check.
	if global, globalErr := gitignore.LoadGlobalPatterns(osfs.New(string(filepath.Separator))); globalErr == nil {
		worktree.Excludes = append(worktree.Excludes, global...)
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, "", err
	}
	if !status.IsClean() {
		return nil, "", fmt.Errorf("%w: affected policies require clean comparison worktrees", errUtils.ErrPolicyInputUnavailable)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, "", err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, "", err
	}
	tree, err := commit.Tree()
	return tree, head.Hash().String(), err
}

func verifyAffectedPolicyRevisions(local, remote *git.Repository, head, base string) error {
	if err := verifyAffectedPolicyRevision(local, head); err != nil {
		return err
	}
	return verifyAffectedPolicyRevision(remote, base)
}

func verifyAffectedPolicyRevision(repo *git.Repository, expected string) error {
	_, current, err := affectedPolicyTree(repo)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("%w: repository changed during affected analysis", errUtils.ErrPolicyInputUnavailable)
	}
	return nil
}

func affectedPolicyFiles(before, after *object.Tree) ([]policy.FileChange, error) {
	// Disable rename inference. A move is a removal and addition, never assumed
	// to preserve a policy attachment or logical instance identity.
	changes, err := object.DiffTreeWithOptions(context.Background(), before, after, &object.DiffTreeOptions{DetectRenames: false})
	if err != nil {
		return nil, err
	}
	files := []policy.FileChange{}
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, err
		}
		file := policy.FileChange{Path: change.To.Name, Action: policy.Modified}
		switch action {
		case merkletrie.Insert:
			file.Action, file.Ambiguous = policy.Added, true
		case merkletrie.Delete:
			file.Path, file.Action, file.Ambiguous = change.From.Name, policy.Removed, true
		case merkletrie.Modify:
			// Includes mode-only and binary changes.
		}
		files = append(files, file)
	}
	return files, nil
}
