package aqua

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// ErrAmbiguousShortName indicates that a short name matched multiple packages in the
// registry index and the caller should disambiguate with the full owner/repo form.
var ErrAmbiguousShortName = errors.New("ambiguous short name")

// shortNameScore ranks index entries against a short-name query.
type shortNameScore int

const (
	scoreNoMatch    shortNameScore = 0
	scoreBinaryOnly shortNameScore = 1 // Binary matches but repo name differs (e.g., kubectl in kubernetes/kubernetes).
	scoreCanonical  shortNameScore = 2 // Binary matches AND repo name matches (e.g., terraform in hashicorp/terraform).
)

// shortNameCandidate is one index entry's match against the short-name query.
type shortNameCandidate struct {
	owner string
	repo  string
	score shortNameScore
}

// ResolveShortName returns the canonical repo_owner/repo_name for a tool's short name
// by consulting the cached aqua-registry index. This mirrors aqua's `aqua g`
// discovery flow: aqua itself has no runtime short-name resolution, so atmos provides
// this UX by searching the index for a package whose binary name matches.
//
// Scoring (highest wins; ties at the top score → ErrAmbiguousShortName):
//   - canonical: repo_name == name AND binary == name (e.g., terraform → hashicorp/terraform).
//   - binary-only: binary == name AND repo_name != name (e.g., kubectl → kubernetes/kubernetes,
//     where the package lives at kubernetes/kubernetes/kubectl).
//
// We scan the full packageList rather than the pathIndex because monorepo packages
// (kubernetes/kubernetes/kubectl, /kubeadm, /kubelet, …) all share one owner/repo
// key and would collide in a map; only the list preserves every binary.
func (ar *AquaRegistry) ResolveShortName(name string) (string, string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.ResolveShortName")()

	if name == "" {
		return "", "", fmt.Errorf("%w: empty short name", registry.ErrInvalidToolSpec)
	}

	pkgs, err := ar.packageInfoList()
	if err != nil {
		return "", "", err
	}

	candidates := scoreShortName(pkgs, name)
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("%w: short name %q not found in registry index", registry.ErrToolNotFound, name)
	}

	// Keep only top-scored candidates. Deduplicate by (owner, repo) so monorepo
	// packages — many binaries sharing one owner/repo — don't trigger false ambiguity:
	// for instance, kubectl, kubeadm, kubelet all live at kubernetes/kubernetes, and
	// asking for kubectl should resolve to the single (kubernetes, kubernetes) repo,
	// not error with "ambiguous, 11 matches" listing the other binaries we didn't ask for.
	top := candidates[0].score
	seen := map[string]bool{}
	tied := candidates[:0]
	for _, c := range candidates {
		if c.score != top {
			break
		}
		k := c.owner + "/" + c.repo
		if seen[k] {
			continue
		}
		seen[k] = true
		tied = append(tied, c)
	}

	if len(tied) == 1 {
		return tied[0].owner, tied[0].repo, nil
	}

	return "", "", ambiguityError(name, tied)
}

// packageInfoList returns a snapshot of the cached package list, loading the
// registry index lazily if necessary.
func (ar *AquaRegistry) packageInfoList() ([]indexPackageInfo, error) {
	ar.pathIndexMu.RLock()
	pkgs := ar.packageList
	ar.pathIndexMu.RUnlock()

	if pkgs != nil {
		return pkgs, nil
	}

	if _, err := ar.fetchRegistryIndex(context.Background()); err != nil {
		return nil, fmt.Errorf("%w: %w", registry.ErrToolNotFound, err)
	}

	ar.pathIndexMu.RLock()
	pkgs = ar.packageList
	ar.pathIndexMu.RUnlock()

	if pkgs == nil {
		return nil, fmt.Errorf("%w: registry index unavailable", registry.ErrToolNotFound)
	}
	return pkgs, nil
}

// scoreShortName walks every index package and returns the ones whose binary
// (or canonical repo) match the short name, sorted by score descending.
func scoreShortName(pkgs []indexPackageInfo, name string) []shortNameCandidate {
	var matches []shortNameCandidate
	for _, p := range pkgs {
		score := classifyShortName(name, p.repo, p.binary)
		if score == scoreNoMatch {
			continue
		}
		matches = append(matches, shortNameCandidate{owner: p.owner, repo: p.repo, score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].owner != matches[j].owner {
			return matches[i].owner < matches[j].owner
		}
		return matches[i].repo < matches[j].repo
	})
	return matches
}

// splitOwnerRepo splits an "owner/repo" key into its parts.
func splitOwnerRepo(key string) (owner, repo string, ok bool) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// binaryFromPath returns the binary name encoded in a registry path. A 3-segment
// path (owner/repo/binary) yields its last segment; a 2-segment path falls back to
// the repo name.
func binaryFromPath(path, fallbackRepo string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return fallbackRepo
}

// classifyShortName scores an index entry against a short-name query.
func classifyShortName(query, repo, binary string) shortNameScore {
	if binary == query && repo == query {
		return scoreCanonical
	}
	if binary == query {
		return scoreBinaryOnly
	}
	return scoreNoMatch
}

// ambiguityError builds a helpful error listing the candidate owner/repo pairs and
// directing the caller to use the full form.
func ambiguityError(name string, tied []shortNameCandidate) error {
	suggestions := make([]string, 0, len(tied))
	for _, c := range tied {
		suggestions = append(suggestions, c.owner+"/"+c.repo)
	}
	return fmt.Errorf("%w: short name %q matches %d packages: %s — use the full owner/repo form",
		ErrAmbiguousShortName, name, len(tied), strings.Join(suggestions, ", "))
}
