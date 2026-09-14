// Package source resolves scaffold templates from local paths or remote
// sources (git, https, s3, oci) into a templates.Configuration ready for
// generation. It is the seam that lets `atmos init`/`atmos scaffold`
// distribute templates remotely while reusing the existing generator engine.
package source

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// DefaultFetchTimeout bounds how long a remote scaffold fetch may take.
const DefaultFetchTimeout = 5 * time.Minute

// refParamPattern matches an existing ref= query parameter, together with
// its leading "?" or "&" separator, in a go-getter git source (e.g.
// "?ref=main" or "&ref=v1").
var refParamPattern = regexp.MustCompile(`([?&])ref=[^&]*`)

// IsTemplateSource reports whether an init/scaffold template argument looks
// like a direct source rather than a catalog or embedded template key.
func IsTemplateSource(value string) bool {
	defer perf.Track(nil, "source.IsTemplateSource")()

	if value == "" {
		return false
	}
	return vendor.IsFileURI(value) ||
		vendor.IsOCIURI(value) ||
		vendor.IsS3URI(value) ||
		vendor.IsGitURI(value) ||
		vendor.HasLocalPathPrefix(value)
}

// queryStart is the separator introducing a URI's first query parameter;
// querySep introduces every subsequent one.
const (
	queryStart = "?"
	querySep   = "&"
)

// appendRefParam appends "ref=ref" to src using queryStart if src has no
// query string yet, or querySep if it does.
func appendRefParam(src, ref string) string {
	sep := queryStart
	if strings.Contains(src, queryStart) {
		sep = querySep
	}
	return src + sep + "ref=" + ref
}

// WithRef applies --ref sugar to a go-getter source. Existing ref query
// parameters win; local paths and file/OCI/S3 sources are returned unchanged.
func WithRef(src, ref string) string {
	defer perf.Track(nil, "source.WithRef")()

	if src == "" || ref == "" {
		return src
	}
	if !vendor.IsGitURI(src) || strings.Contains(src, "ref=") {
		return src
	}
	return appendRefParam(src, ref)
}

// replaceRef force-overrides src's existing ref= query parameter with ref,
// appending one if none exists. Unlike WithRef (which intentionally leaves
// an existing ref= alone -- that's the sugar for --ref not clobbering a
// source that's already pinned), this always wins: it backs
// pinRenderedRef's need to override a mutable tag/branch (e.g. a recorded
// source's own "?ref=main") with the resolved, immutable commit SHA, so a
// moving branch can't change what --update-strategy=rendered's merge base
// resolves to.
func replaceRef(src, ref string) string {
	if src == "" || ref == "" || !vendor.IsGitURI(src) {
		return src
	}
	if refParamPattern.MatchString(src) {
		return refParamPattern.ReplaceAllString(src, "${1}ref="+ref)
	}
	return appendRefParam(src, ref)
}

// pinRenderedRef re-expresses src as a request to fetch exactly renderedRef
// -- a resolved commit SHA for git sources, an OCI manifest digest for OCI
// sources (see resolveFetchedGitRef/resolveOCI's respective capture) --
// overriding whatever mutable ref src's own tag/branch currently names. Git
// sources go through replaceRef, not WithRef: WithRef intentionally leaves
// an existing ?ref= alone (that's the sugar for --ref not clobbering an
// already-pinned source), which would silently keep src's original mutable
// ref= here and defeat the whole point of pinning to renderedRef. OCI's
// immutable pin is expressed differently (an explicit @digest), so this
// dispatches on source kind rather than folding OCI into replaceRef itself.
// WithRef (unlike replaceRef) also serves --ref's CLI flag, where the value
// is a tag/branch name, not a digest -- conflating the two would let a plain
// --ref value reach Repository.Digest and fail (or worse, silently
// misresolve) instead of the CLI flag's existing "--ref is ignored for OCI"
// behavior.
func pinRenderedRef(src, renderedRef string) (string, error) {
	defer perf.Track(nil, "source.pinRenderedRef")()

	if src == "" || renderedRef == "" {
		return src, nil
	}
	if vendor.IsOCIURI(src) {
		pinned, err := oci.PinDigest(strings.TrimPrefix(src, "oci://"), renderedRef)
		if err != nil {
			return "", err
		}
		return "oci://" + pinned, nil
	}
	return replaceRef(src, renderedRef), nil
}

// UnpinnedRenderedRefMarker is recorded as spec.renderedRef for
// --update-strategy=rendered generations whose source has no immutable ref
// to pin at all (see IsPinnableSource: local path, file://, s3::, or a plain
// http(s) archive). Resolve legitimately leaves Configuration.ResolvedRef
// empty for those source kinds, but SaveProjectRecord only ever persists a
// non-empty spec.renderedRef, and ResolveRenderedBase/CheckNotSwitchedFromRendered
// both key off spec.renderedRef being non-empty to recognize "this project
// was generated under rendered" -- an empty ResolvedRef there would silently
// make the record look exactly like one that was never generated under
// rendered at all, breaking both a later rendered update (which would wrongly
// fail with "no recorded rendered-strategy history") and a later tracked
// update (which would wrongly skip CheckNotSwitchedFromRendered's guard and
// attempt a 3-way merge against stale or absent git history).
//
// Recording it is safe: pinRenderedRef leaves any non-git/non-oci src
// completely unchanged regardless of the ref passed to it (replaceRef's own
// !vendor.IsGitURI(src) no-op), so recording this marker for a non-pinnable
// source never corrupts the source that ResolveRenderedBase re-fetches --
// it only exists to keep spec.renderedRef non-empty for those two checks.
const UnpinnedRenderedRefMarker = "unpinned"

// IsPinnableSource reports whether src is a source kind for which Resolve
// records an immutable ResolvedRef (a git:: commit SHA or an oci:// manifest
// digest -- see resolveFetchedGitRef/resolveSubdirGitRef and resolveOCI's
// respective captures). Local paths, file://, s3::, and plain http(s)
// archive sources have no equivalent immutable identity to pin to, so
// Resolve legitimately leaves ResolvedRef empty for them; this distinguishes
// that expected, by-design case from an unresolved git/oci ref, which
// instead signals a resolution failure that callers should not paper over
// with UnpinnedRenderedRefMarker.
func IsPinnableSource(src string) bool {
	defer perf.Track(nil, "source.IsPinnableSource")()

	return vendor.IsGitURI(src) || vendor.IsOCIURI(src)
}

// Resolve fetches a scaffold template from src (a local path, file://, an
// oci:// registry reference, or a go-getter remote such as git/https/s3)
// into a usable templates.Configuration. The returned cleanup function
// removes any temporary download directory; it is never nil and is always
// safe to call.
func Resolve(atmosConfig *schema.AtmosConfiguration, name, src string, timeout time.Duration) (*templates.Configuration, func(), error) {
	defer perf.Track(nil, "source.Resolve")()

	noop := func() {}
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}

	// Local sources (relative/absolute path or file://) load directly, no fetch.
	if vendor.IsFileURI(src) || vendor.IsLocalPath(src) {
		conf, err := resolveLocal(name, src)
		return conf, noop, err
	}

	// OCI sources pull directly via pkg/oci -- OCI isn't a go-getter scheme
	// (see pkg/provisioner/source/vendor.go's downloadOCISource, which this
	// mirrors), so it's handled before the go-getter branch below.
	if vendor.IsOCIURI(src) {
		return resolveOCI(atmosConfig, name, src, timeout)
	}

	// Remote sources: download into a temp dir via go-getter, then load.
	return resolveRemote(atmosConfig, name, src, timeout)
}

// resolveOCI pulls a scaffold template from an oci:// registry reference
// into a temporary directory and loads its configuration. Mirrors
// resolveRemote's temp-dir/cleanup/provenance shape, but fetches via
// pkg/oci.ProcessImage (the same primitive atmos vendor pull and JIT
// component-source provisioning already use) instead of go-getter, since
// OCI registries aren't a go-getter scheme. The returned cleanup function
// removes the temporary directory.
func resolveOCI(atmosConfig *schema.AtmosConfiguration, name, src string, timeout time.Duration) (*templates.Configuration, func(), error) {
	noop := func() {}

	tempDir, err := os.MkdirTemp("", "atmos-scaffold-")
	if err != nil {
		return nil, noop, errUtils.Build(errUtils.ErrCreateTempDirectory).
			WithCause(err).
			WithExplanation("Failed to create a temporary directory for the scaffold download").
			WithExitCode(1).
			Err()
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	imageRef := strings.TrimPrefix(src, "oci://")

	// Resolve the immutable manifest digest *before* fetching, and fetch
	// that exact digest reference rather than imageRef's own (possibly
	// moving) tag/branch, so the digest recorded below for
	// --update-strategy=rendered's commit-pinning always identifies the
	// very manifest whose layers were extracted into tempDir -- not a
	// second, potentially different manifest a tag happened to point to by
	// the time a separate post-fetch resolution ran. Best-effort: a
	// resolution failure here falls back to fetching imageRef directly
	// (today's pre-fix behavior, mirroring resolveFetchedGitRef's own
	// best-effort git-side resolution), since it only feeds rendered mode's
	// optional pinning, never the fetch itself.
	fetchRef := imageRef
	resolvedDigest := ""
	if resolved, resolveErr := oci.ResolveImage(ctx, atmosConfig, imageRef); resolveErr == nil {
		resolvedDigest = resolved.Digest
		if pinned, pinErr := oci.PinDigest(imageRef, resolved.Digest); pinErr == nil {
			fetchRef = pinned
		}
	}

	if err := oci.ProcessImage(ctx, atmosConfig, fetchRef, tempDir); err != nil {
		cleanup()
		return nil, noop, errUtils.Build(errUtils.ErrScaffoldFetchSource).
			WithCause(err).
			WithExplanationf("Failed to fetch scaffold template from `%s`", src).
			WithHint("Verify the OCI reference is correct and accessible").
			WithHint("For private registries, run `docker login`, or set `ATMOS_GITHUB_TOKEN` for ghcr.io").
			WithContext("source", src).
			WithExitCode(1).
			Err()
	}

	conf, err := templates.LoadConfigurationFromDir(name, tempDir)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	if err := requireScaffoldConfig(conf, src); err != nil {
		cleanup()
		return nil, noop, err
	}
	conf.ResolvedRef = resolvedDigest
	// tempDir only exists to read files off disk and is removed by cleanup()
	// once generation finishes; the recorded provenance must be the original
	// source the caller passed in, not that ephemeral fetch destination.
	conf.Source = src
	return conf, cleanup, nil
}

// resolveRemote fetches a remote scaffold source (git/https/s3, via
// go-getter) into a temporary directory and loads its configuration. The
// returned cleanup function removes the temporary directory.
func resolveRemote(atmosConfig *schema.AtmosConfiguration, name, src string, timeout time.Duration) (*templates.Configuration, func(), error) {
	noop := func() {}

	// For a //subdir git source, resolve and pin the commit *before* the
	// content fetch below, rather than after it (see pinSubdirGitSource's
	// doc comment). Resolving afterward via a second, separate fetch let a
	// mutable ref (a moving branch/tag) select a different commit for that
	// second fetch than the one the first, content-bearing fetch already
	// used, so the generated files could come from commit A while
	// conf.ResolvedRef recorded commit B -- a later --update-strategy=rendered
	// update would then pin B and diff against the wrong three-way merge
	// base. Pinning fetchSrc to the pre-resolved SHA makes the single fetch
	// below and preResolvedRef always agree.
	fetchSrc, preResolvedRef := pinSubdirGitSource(atmosConfig, src, timeout)

	tempDir, err := os.MkdirTemp("", "atmos-scaffold-")
	if err != nil {
		return nil, noop, errUtils.Build(errUtils.ErrCreateTempDirectory).
			WithCause(err).
			WithExplanation("Failed to create a temporary directory for the scaffold download").
			WithExitCode(1).
			Err()
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	if err := fetchRemoteSource(atmosConfig, name, fetchSrc, tempDir, timeout); err != nil {
		cleanup()
		return nil, noop, err
	}

	conf, err := templates.LoadConfigurationFromDir(name, tempDir)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	if err := requireScaffoldConfig(conf, src); err != nil {
		cleanup()
		return nil, noop, err
	}
	// Resolve while tempDir (and its .git, if src was a git:: source with no
	// //subdir) still exists -- cleanup() below removes it once generation
	// finishes.
	conf.ResolvedRef = resolveFetchedGitRef(tempDir)
	if conf.ResolvedRef == "" {
		// Either src wasn't a git source at all, or it was a //subdir git
		// source whose fetch never leaves a usable .git in tempDir (see
		// pinSubdirGitSource's doc comment) -- fall back to whatever commit
		// pinSubdirGitSource already resolved, and pinned fetchSrc to,
		// before the fetch above ran.
		conf.ResolvedRef = preResolvedRef
	}
	// tempDir only exists to read files off disk and is removed by cleanup()
	// once generation finishes; the recorded provenance must be the original
	// source the caller passed in, not that ephemeral fetch destination.
	conf.Source = src
	return conf, cleanup, nil
}

// resolveFetchedGitRef returns the commit SHA checked out at dir, or "" if
// dir isn't a git working tree (src wasn't a git:: source, e.g. oci/s3/http).
// Best-effort: any resolution failure is treated the same as "not a git
// source" rather than failing the whole fetch, since the result only feeds
// --update-strategy=rendered's optional commit-pinning, never the fetch
// itself.
func resolveFetchedGitRef(dir string) string {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	return head.Hash().String()
}

// pinSubdirGitSource resolves and pins the commit for a //subdir git source
// *before* resolveRemote's own content fetch runs, so that fetch and the
// commit recorded in conf.ResolvedRef can never split across two different
// commits of a mutable ref. This matters because go-getter's git fetch for a
// //subdir source clones the full repository into its own internal temp
// location first, then copies only the subdir's content into the
// destination -- see pkg/downloader/get_git.go's doc comment -- so the
// destination itself never gets a usable .git directory for
// resolveFetchedGitRef to inspect afterward. Resolving post-fetch therefore
// requires a second, separate fetch of the same ref; if that ref is mutable
// (a moving branch/tag), the second fetch can select a different commit
// than the first one already used for content, silently mismatching the
// two.
//
// Resolving first instead -- via resolveSubdirGitRef -- and then pinning
// fetchSrc to that exact SHA (via replaceRef) makes the single content fetch
// that follows always land on the very commit being recorded.
//
// Returns the (possibly re-pinned) source to fetch and the commit SHA it was
// pinned to. If src isn't a git source, has no //subdir, or the resolution
// itself fails, returns src unchanged and an empty ref -- the caller falls
// back to resolveFetchedGitRef's post-fetch, best-effort resolution in that
// case (e.g. a non-git source, or a subdir-less git source where tempDir's
// own .git is directly inspectable).
func pinSubdirGitSource(atmosConfig *schema.AtmosConfiguration, src string, timeout time.Duration) (string, string) {
	defer perf.Track(nil, "source.pinSubdirGitSource")()

	if !vendor.IsGitURI(src) {
		return src, ""
	}
	_, subdir := getter.SourceDirSubdir(src)
	if subdir == "" {
		return src, ""
	}
	ref := resolveSubdirGitRef(atmosConfig, src, timeout)
	if ref == "" {
		return src, ""
	}
	return replaceRef(src, ref), ref
}

// resolveSubdirGitRef re-fetches src's git ref without its //subdir suffix
// into a throwaway temp directory, purely to resolve the commit checked out
// there via resolveFetchedGitRef -- see pinSubdirGitSource's call site for
// why this is needed. Returns "" if src has no //subdir at all (the caller's
// direct resolveFetchedGitRef(tempDir) result already reflects reality in
// that case) or if the re-fetch itself fails.
func resolveSubdirGitRef(atmosConfig *schema.AtmosConfiguration, src string, timeout time.Duration) string {
	rootSrc, subdir := getter.SourceDirSubdir(src)
	if subdir == "" {
		return ""
	}

	tempDir, err := os.MkdirTemp("", "atmos-scaffold-gitref-")
	if err != nil {
		return ""
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	if err := fetchRemoteSource(atmosConfig, "gitref-probe", rootSrc, tempDir, timeout); err != nil {
		return ""
	}
	return resolveFetchedGitRef(tempDir)
}

// fetchRemoteSource downloads src into destDir via go-getter, showing a
// spinner while the download runs.
func fetchRemoteSource(atmosConfig *schema.AtmosConfiguration, name, src, destDir string, timeout time.Duration) error {
	normalized := vendor.NormalizeURI(src)
	// Keep the spinner message short: the full go-getter URL (subdir + ref)
	// can be long enough to wrap across terminal rows, which breaks
	// bubbletea's in-place redraw and makes the spinner scroll a new line
	// per tick instead of overwriting.
	progressMsg := fmt.Sprintf("Fetching scaffold template `%s`", name)
	completedMsg := fmt.Sprintf("Fetched scaffold template `%s`", name)
	fetchErr := spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		return downloader.NewGoGetterDownloader(atmosConfig).Fetch(normalized, destDir, downloader.ClientModeDir, timeout)
	})
	if fetchErr != nil {
		return errUtils.Build(errUtils.ErrScaffoldFetchSource).
			WithCause(fetchErr).
			WithExplanationf("Failed to fetch scaffold template from `%s`", src).
			WithHint("Check the source URL and your network connection").
			WithHint("For private repositories set `ATMOS_GITHUB_TOKEN` (or the host-specific token)").
			WithContext("source", src).
			WithExitCode(1).
			Err()
	}
	return nil
}

func resolveLocal(name, src string) (*templates.Configuration, error) {
	path := strings.TrimPrefix(src, "file://")
	conf, err := templates.LoadConfigurationFromDir(name, path)
	if err != nil {
		return nil, err
	}
	if err := requireScaffoldConfig(conf, src); err != nil {
		return nil, err
	}
	return conf, nil
}

func requireScaffoldConfig(conf *templates.Configuration, src string) error {
	if conf != nil && templates.HasScaffoldConfig(conf.Files) {
		return nil
	}
	return errUtils.Build(errUtils.ErrScaffoldConfigMissing).
		WithExplanationf("Template source `%s` does not contain scaffold.yaml at its root", src).
		WithHint("Point the source at a scaffold template directory, or use go-getter //subdir syntax").
		WithContext("source", src).
		WithExitCode(2).
		Err()
}

// Hydrate materializes a catalog/remote stub (a Configuration with no Files but
// a Source) into a full template by fetching its Source. Full templates
// (embedded or already-loaded local) are returned unchanged with a no-op
// cleanup. The returned cleanup must be called after generation completes.
func Hydrate(stub *templates.Configuration, override string) (func(), error) {
	defer perf.Track(nil, "source.Hydrate")()

	noop := func() {}
	if len(stub.Files) > 0 || stub.Source == "" {
		return noop, nil
	}

	// Only remote sources need the downloader (and thus a loaded config for
	// token injection). Local/override sources resolve without touching config.
	var atmosConfig schema.AtmosConfiguration
	if !vendor.IsLocalPath(stub.Source) && !vendor.IsFileURI(stub.Source) {
		loaded, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return noop, err
		}
		atmosConfig = loaded
	}

	resolved, cleanup, err := Resolve(&atmosConfig, stub.Name, stub.Source, DefaultFetchTimeout)
	if err != nil {
		return noop, err
	}
	*stub = *resolved
	return cleanup, nil
}
