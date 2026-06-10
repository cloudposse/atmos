package git

import (
	"strings"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// argKind classifies a positional argument as a configured repository name,
// a URI, or a filesystem path.
type argKind int

const (
	argKindName argKind = iota
	argKindURI
	argKindPath
)

// classifyArg determines how to interpret a positional argument:
//
//  1. A configured repository name (exact match in git.repositories) wins.
//  2. A "./" prefix forces path interpretation.
//  3. A URI-shaped string (https://, git@, git::, …) is treated as a URI.
//  4. Anything else is treated as a filesystem path.
func classifyArg(arg string, cfg *schema.GitConfig) argKind {
	if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "/") {
		return argKindPath
	}
	if cfg != nil {
		if _, ok := cfg.Repositories[arg]; ok {
			return argKindName
		}
	}
	if IsURI(arg) {
		return argKindURI
	}
	return argKindPath
}

// resolveRepoByName looks up a repository by name and returns a ResolvedRepository.
func resolveRepoByName(name string, cfg *schema.GitConfig) (*atmosgit.ResolvedRepository, error) {
	return atmosgit.ResolveRepository(cfg, name)
}
