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
		vendor.IsNonGitHTTPURI(value) ||
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
// sources (see fetchRemoteSource/fetchOCI's respective capture) --
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
// digest -- see fetchRemoteSource and fetchOCI's
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

	directory, cleanup, err := FetchDirectory(atmosConfig, name, src, timeout)
	if err != nil {
		return nil, cleanup, err
	}
	conf, err := directory.LoadScaffold(name, src)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return conf, cleanup, nil
}

// fetchOCI pulls a source directory from an oci:// registry reference
// into a temporary directory without interpreting its contents. Mirrors
// fetchRemoteDirectory's temp-dir/cleanup/provenance shape, but fetches via
// pkg/oci.ProcessImage (the same primitive atmos vendor pull and JIT
// component-source provisioning already use) instead of go-getter, since
// OCI registries aren't a go-getter scheme. The returned cleanup function
// removes the temporary directory.
func fetchOCI(atmosConfig *schema.AtmosConfiguration, src string, timeout time.Duration) (*Directory, func(), error) {
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
	// (mirroring the best-effort Git provenance capture), since it only feeds rendered mode's
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

	return &Directory{Path: tempDir, ResolvedRef: resolvedDigest}, cleanup, nil
}

// fetchRemoteDirectory fetches a remote source (git/https/s3, via
// go-getter) into a temporary directory without interpreting its contents. The
// returned cleanup function removes the temporary directory.
func fetchRemoteDirectory(atmosConfig *schema.AtmosConfiguration, name, src string, timeout time.Duration, options *fetchOptions) (*Directory, func(), error) {
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

	metadata, err := options.fetchRemoteSource(atmosConfig, name, src, tempDir, timeout)
	if err != nil {
		cleanup()
		return nil, noop, err
	}

	return &Directory{Path: tempDir, ResolvedRef: metadata.GitCommit}, cleanup, nil
}

// fetchRemoteSource downloads src into destDir via go-getter, showing a
// spinner while the download runs.
func (options *fetchOptions) fetchRemoteSource(atmosConfig *schema.AtmosConfiguration, name, src, destDir string, timeout time.Duration) (downloader.FetchMetadata, error) {
	normalized := vendor.NormalizeURI(src)
	// Keep the spinner message short: the full go-getter URL (subdir + ref)
	// can be long enough to wrap across terminal rows, which breaks
	// bubbletea's in-place redraw and makes the spinner scroll a new line
	// per tick instead of overwriting.
	progressMsg := fmt.Sprintf("Fetching source `%s`", name)
	completedMsg := fmt.Sprintf("Fetched source `%s`", name)
	var metadata downloader.FetchMetadata
	fetchErr := options.run(progressMsg, completedMsg, func() error {
		var err error
		metadata, err = downloader.NewGoGetterDownloader(atmosConfig).FetchWithMetadata(normalized, destDir, downloader.ClientModeDir, timeout)
		return err
	})
	if fetchErr != nil {
		return downloader.FetchMetadata{}, errUtils.Build(errUtils.ErrScaffoldFetchSource).
			WithCause(fetchErr).
			WithExplanationf("Failed to fetch scaffold template from `%s`", src).
			WithHint("Check the source URL and your network connection").
			WithHint("For private repositories set `ATMOS_GITHUB_TOKEN` (or the host-specific token)").
			WithContext("source", src).
			WithExitCode(1).
			Err()
	}
	return metadata, nil
}

// run leaves a shared indicator running for the next initialization step.
func (options *fetchOptions) run(message, completed string, operation func() error) error {
	if options.progress == nil {
		return spinner.ExecWithSpinner(message, completed, operation)
	}
	options.progress.Update(message)
	return operation()
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
