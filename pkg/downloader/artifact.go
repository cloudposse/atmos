//nolint:gocognit,revive // Resolution and deterministic tree hashing must keep the source-type decision tree together.
package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	gitSHA1Length   = 40
	gitSHA256Length = 64
)

var errInvalidGitCommit = errors.New("invalid git commit")

// ResolvedArtifact is immutable provenance captured at the common download
// boundary. Declared and Resolved never contain URL credentials or queries.
// Cache metadata deliberately has no role in integrity verification.
type ResolvedArtifact struct {
	Declared string
	Resolved string
	Kind     string
	Identity string
	ETag     string
	Modified string
}

// ResolveArtifact derives the strongest available immutable identity for an
// already staged artifact. OCI sources resolve to the registry manifest digest;
// Git staging directories resolve to HEAD; all other staged/local directories
// use a deterministic content-tree SHA-256 fallback.
func ResolveArtifact(ctx context.Context, config *schema.AtmosConfiguration, declared, stagedPath string) (ResolvedArtifact, error) {
	artifact := ResolvedArtifact{Declared: RedactSource(declared), Resolved: RedactSource(declared), Kind: "archive"}
	if strings.HasPrefix(declared, "oci://") {
		resolved, err := oci.ResolveImage(ctx, config, strings.TrimPrefix(declared, "oci://"))
		if err != nil {
			return artifact, err
		}
		artifact.Kind = "oci"
		artifact.Resolved = resolved.Reference
		artifact.Identity = resolved.Digest
		return artifact, nil
	}
	if digest := digestFromReference(declared); digest != "" {
		artifact.Identity = digest
		return artifact, nil
	}
	if info, err := os.Stat(declared); err == nil && info.IsDir() {
		artifact.Kind = "local"
	}
	if stagedPath != "" {
		if commit, err := gitCommit(stagedPath); err == nil {
			artifact.Kind = "git"
			artifact.Identity = commit
			return artifact, nil
		}
		identity, err := TreeSHA256(stagedPath)
		if err != nil {
			return artifact, err
		}
		artifact.Identity = identity
		return artifact, nil
	}
	if info, err := os.Stat(declared); err == nil && info.IsDir() {
		identity, hashErr := TreeSHA256(declared)
		if hashErr != nil {
			return artifact, hashErr
		}
		artifact.Identity = identity
	}
	return artifact, nil
}

// TreeSHA256 hashes a directory's portable file manifest and bytes. It is the
// fallback identity for archives and local paths, never a replacement for a
// stronger Git or OCI identity.
func TreeSHA256(root string) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk artifact %q: %w", root, err)
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		// This hashes a portable artifact manifest for immutable content identity,
		// never a password or credential.
		// codeql[go/weak-sensitive-data-hashing]
		_, _ = fmt.Fprintf(hash, "%s\x00%o\x00", filepath.ToSlash(rel), info.Mode().Perm())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			_, _ = io.WriteString(hash, target)
			_, _ = io.WriteString(hash, "\n")
			continue
		}
		file, err := os.Open(path) // #nosec G304 -- path is discovered under caller-owned root.
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		_, _ = io.WriteString(hash, "\n")
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// RedactSource removes credentials and query strings before a source reaches a
// lock, workdir receipt, log, or SBOM.
func RedactSource(source string) string {
	if parsed, err := url.Parse(source); err == nil && parsed.Scheme != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String()
	}
	return strings.SplitN(source, "?", 2)[0]
}

func gitCommit(directory string) (string, error) {
	output, err := osExec.Command("git", "-C", directory, "rev-parse", "HEAD").Output() // #nosec G204 -- fixed command and caller-owned staging directory.
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(output))
	if len(commit) != gitSHA1Length && len(commit) != gitSHA256Length {
		return "", errInvalidGitCommit
	}
	return commit, nil
}

func digestFromReference(source string) string {
	if index := strings.LastIndex(source, "@sha256:"); index >= 0 {
		return source[index+1:]
	}
	return ""
}
