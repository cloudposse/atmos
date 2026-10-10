package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFetcherRejectsInvalidBundledNamesAndMissingContent(t *testing.T) {
	for _, name := range []string{"../atmos-terraform", "..", "does-not-exist"} {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(realTemp(t), "snapshot")
			f := &GitFetcher{Config: &schema.AtmosConfiguration{}}
			_, err := f.Fetch(context.Background(), Repository{Source: "bundled:" + name}, dest)
			require.Error(t, err)
			require.NoDirExists(t, dest)
		})
	}
}

func TestGitFetcherRejectsUnusableCommitEvidence(t *testing.T) {
	for _, kind := range []string{"invalid commit", "metadata missing archive", "metadata missing destination"} {
		t.Run(kind, func(t *testing.T) {
			dest := filepath.Join(realTemp(t), "snapshot")
			// No Git executable is required: a non-Git tree must never be accepted as a pinned repository.
			t.Setenv("PATH", realTemp(t))
			f := &GitFetcher{Config: &schema.AtmosConfiguration{}, Downloader: downloadStub{fetch: func(context.Context, string, string) (downloader.FetchMetadata, error) {
				if kind == "invalid commit" {
					return downloader.FetchMetadata{GitCommit: "moving-branch"}, nil
				}
				if kind == "metadata missing archive" {
					write(t, filepath.Join(dest, "README.md"), "downloaded archive")
				}
				return downloader.FetchMetadata{}, nil
			}}}
			_, err := f.Fetch(context.Background(), Repository{Source: "org/repo"}, dest)
			switch kind {
			case "invalid commit":
				require.ErrorIs(t, err, ErrInvalid)
				require.ErrorContains(t, err, "invalid resolved Git commit")
			case "metadata missing archive":
				require.ErrorIs(t, err, ErrInvalid)
				require.ErrorContains(t, err, "did not resolve to Git")
			default:
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestGitSourceRejectsMalformedAddressesBeforeDownload(t *testing.T) {
	for _, source := range []string{"git@no-path", "https://host/%zz", "ftp://host/repo"} {
		t.Run(source, func(t *testing.T) {
			f := &GitFetcher{Config: &schema.AtmosConfiguration{}, Downloader: downloadStub{fetch: func(context.Context, string, string) (downloader.FetchMetadata, error) {
				t.Fatal("invalid source must not contact a downloader")
				return downloader.FetchMetadata{}, nil
			}}}
			_, err := f.Fetch(context.Background(), Repository{Source: source}, filepath.Join(realTemp(t), "snapshot"))
			require.Error(t, err)
		})
	}
}
