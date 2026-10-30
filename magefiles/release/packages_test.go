package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func packageFixtureAssets() (map[string]string, map[string][]byte) {
	assets := map[string]string{}
	contents := map[string][]byte{}
	for _, platform := range []string{"linux_amd64", "linux_arm64"} {
		data := []byte("verified binary for " + platform)
		sum := sha256.Sum256(data)
		assets["atmos_1.234.0_"+platform] = hex.EncodeToString(sum[:])
		for _, format := range []string{".deb", ".rpm", ".apk"} {
			name := "atmos_1.234.0_" + platform + format
			assets[name] = "package digest already verified"
			contents[name] = data
		}
	}
	assets["metadata.json"] = "nonpackage metadata"
	return assets, contents
}

func TestVerifyPackagesCompleteMatrixMatchesVerifiedBytes(t *testing.T) {
	dir := t.TempDir()
	assets, contents := packageFixtureAssets()
	extracted := map[string]int{}
	err := verifyPackages(context.Background(), dir, "1.234.0", assets, func(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
		require.NoError(t, ctx.Err())
		require.Equal(t, "bsdtar", name)
		switch args[0] {
		case "-tf":
			require.Len(t, args, 2)
			require.Nil(t, input)
			require.Equal(t, dir, filepath.Dir(args[1]))
			require.Equal(t, ".deb", filepath.Ext(args[1]))
			return []byte("debian-binary\ncontrol.tar.gz\ndata.tar.xz\n"), nil
		case "-xOf":
			require.Len(t, args, 3)
			require.Nil(t, input)
			require.Equal(t, "data.tar.xz", args[2])
			return []byte(filepath.Base(args[1])), nil
		default:
			require.Equal(t, "--ignore-zeros", args[0])
			archive := args[2]
			if archive == "-" {
				archive = filepath.Join(dir, string(input))
				require.Equal(t, ".deb", filepath.Ext(archive))
			} else {
				require.Nil(t, input)
			}
			require.Equal(t, dir, filepath.Dir(archive))
			key := filepath.Base(archive)
			data, ok := contents[key]
			require.True(t, ok)
			member := "./usr/bin/atmos"
			if filepath.Ext(archive) == ".rpm" {
				member = "/usr/bin/atmos" // GoReleaser's nFPM RPM headers use absolute paths.
			}
			if args[1] == "-tf" {
				require.Len(t, args, 3)
				return []byte("./usr/\n./usr/bin/\n" + member + "\nusr/share/doc/atmos\n"), nil
			}
			require.Equal(t, []string{"--ignore-zeros", "-xOf", args[2], member}, args)
			extracted[key]++
			return data, nil
		}
	})
	require.NoError(t, err)
	require.Len(t, extracted, 6)
	for name := range contents {
		require.Equal(t, 1, extracted[name], name)
	}
}

func TestVerifyPackagesRejectMalformedArchivesAndMismatchedBinaries(t *testing.T) {
	toolErr := errors.New("archive parser failure")
	for _, mode := range []string{"Debian list failure", "Debian missing data", "Debian duplicate data", "Debian extraction failure", "package list failure", "missing executable", "duplicate executable", "extraction failure", "wrong binary", "duplicate platform", "missing package"} {
		t.Run(mode, func(t *testing.T) {
			assets, contents := packageFixtureAssets()
			if strings.HasPrefix(mode, "Debian") {
				for name := range contents {
					if !strings.HasSuffix(name, ".deb") {
						delete(assets, name)
					}
				}
			}
			if mode == "duplicate platform" {
				assets["duplicate.apk"] = "digest"
				contents["duplicate.apk"] = contents["atmos_1.234.0_linux_amd64.apk"]
			}
			if mode == "missing package" {
				delete(assets, "atmos_1.234.0_linux_arm64.deb")
			}
			err := verifyPackages(context.Background(), t.TempDir(), "1.234.0", assets, func(_ context.Context, input []byte, name string, args ...string) ([]byte, error) {
				require.Equal(t, "bsdtar", name)
				switch args[0] {
				case "-tf":
					switch mode {
					case "Debian list failure":
						return nil, toolErr
					case "Debian missing data":
						return []byte("debian-binary\ncontrol.tar.gz\n"), nil
					case "Debian duplicate data":
						return []byte("data.tar.gz\ndata.tar.xz\n"), nil
					}
					return []byte("data.tar.gz\n"), nil
				case "-xOf":
					if mode == "Debian extraction failure" {
						return nil, toolErr
					}
					return []byte(filepath.Base(args[1])), nil
				default:
					archive := filepath.Base(args[2])
					if args[2] == "-" {
						archive = string(input)
					}
					if args[1] == "-tf" {
						switch mode {
						case "package list failure":
							return nil, toolErr
						case "missing executable":
							return []byte("usr/share/atmos\n"), nil
						case "duplicate executable":
							return []byte("usr/bin/atmos\n/usr/bin/atmos\n"), nil
						}
						return []byte("usr/bin/atmos\n"), nil
					}
					if mode == "extraction failure" {
						return nil, toolErr
					}
					if mode == "wrong binary" {
						return []byte("wrong commit despite valid package checksum"), nil
					}
					return contents[archive], nil
				}
			})
			require.Error(t, err)
			if strings.Contains(mode, "failure") {
				require.ErrorIs(t, err, toolErr)
			} else {
				require.ErrorIs(t, err, ErrReleaseVerification)
			}
			switch mode {
			case "wrong binary":
				require.ErrorContains(t, err, "unverified executable")
			case "duplicate platform":
				require.ErrorContains(t, err, "duplicate package")
			case "missing package":
				require.ErrorContains(t, err, "missing .deb package for linux_arm64")
			}
		})
	}
}

func TestVerifyBuildStopsBeforePackageParsingOnInvalidBuild(t *testing.T) {
	dir := t.TempDir()
	const tag = "v1.234.0"
	commit := strings.Repeat("a", 40)
	require.Error(t, (Release{}).VerifyBuild(dir, tag, commit))
	metadata, err := json.Marshal(releaseMetadata{Tag: tag, Version: tag[1:], Commit: commit})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), metadata, 0o600))
	require.Error(t, (Release{}).VerifyBuild(dir, tag, commit))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated"), []byte("bytes"), 0o600))
	digest, err := digestFile(filepath.Join(dir, "unrelated"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos_1.234.0_SHA256SUMS"), []byte(digest+"  unrelated\n"), 0o600))
	require.ErrorContains(t, (Release{}).VerifyBuild(dir, tag, commit), "missing binary")
}

func TestRunReleaseTool(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("ATMOS_RELEASE_TOOL_TEST", "success")
	output, err := runReleaseTool(context.Background(), []byte("stdin bytes"), executable, "-test.run=^TestReleaseToolHelperProcess$", "--", "literal $(not-executed)")
	require.NoError(t, err)
	require.Equal(t, "stdin bytes|literal $(not-executed)", string(output))
	t.Setenv("ATMOS_RELEASE_TOOL_TEST", "failure")
	_, err = runReleaseTool(context.Background(), nil, executable, "-test.run=^TestReleaseToolHelperProcess$")
	require.ErrorContains(t, err, "parser stderr")
	require.ErrorContains(t, err, "exit status 7")
}

func TestReleaseToolHelperProcess(t *testing.T) {
	mode := os.Getenv("ATMOS_RELEASE_TOOL_TEST")
	if mode == "" {
		return
	}
	if mode == "failure" {
		fmt.Fprint(os.Stderr, "parser stderr")
		os.Exit(7)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(8)
	}
	fmt.Printf("%s|%s", data, os.Args[len(os.Args)-1])
	os.Exit(0)
}
