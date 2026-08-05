package downloader

import (
	"net/url"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func fakeAtmosConfig() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			InjectGithubToken: false,
		},
	}
}

func TestRewriteSCPURL_NoMatch(t *testing.T) {
	nonSCP := "not-an-scp-url"
	_, rewritten := rewriteSCPURL(nonSCP)
	if rewritten {
		t.Error("Expected non-SCP URL not to be rewritten")
	}
}

func TestRewriteSCPURL(t *testing.T) {
	scp := "git@github.com:user/repo.git"
	newURL, rewritten := rewriteSCPURL(scp)
	if !rewritten {
		t.Errorf("Expected SCP URL to be rewritten")
	}
	if !strings.HasPrefix(newURL, "ssh://") {
		t.Errorf("Expected rewritten URL to start with ssh://, got %s", newURL)
	}
	nonSCP := "https://github.com/user/repo.git"
	_, ok := rewriteSCPURL(nonSCP)
	if ok {
		t.Error("Expected non-SCP URL to not be rewritten")
	}
}

func TestNormalizePath_ErrorHandling(t *testing.T) {
	uObj := &url.URL{
		Scheme: "http",
		Host:   "example.com",
		Path:   "%zz",
	}
	(&CustomGitDetector{}).normalizePath(uObj)
	if uObj.Path == "" {
		t.Error("Expected normalized path to be non-empty even on unescape error")
	}
	if uObj.Path != "%zz" {
		t.Errorf("Expected path to remain unchanged on error, got %s", uObj.Path)
	}
}

func TestDetect_LocalFilePath(t *testing.T) {
	// This tests the branch when the input is a local file path (no host).
	config := fakeAtmosConfig()
	detector := &CustomGitDetector{atmosConfig: &config, source: "/home/user/repo"}
	localFile := "/home/user/repo/README.md"
	result, ok, err := detector.Detect(localFile, "")
	if err != nil {
		t.Fatalf("Expected no error for local file path, got: %v", err)
	}
	if ok != false {
		t.Errorf("Expected ok to be false for local file path, got true")
	}
	if result != "" {
		t.Errorf("Expected result to be empty for local file path, got: %s", result)
	}
}

func TestEnsureScheme(t *testing.T) {
	config := fakeAtmosConfig()
	detector := &CustomGitDetector{atmosConfig: &config}
	in := "https://example.com/repo.git"
	out := detector.ensureScheme(in)
	if !strings.HasPrefix(out, "https://") {
		t.Errorf("Expected scheme to be preserved, got %s", out)
	}
	scp := "git@github.com:user/repo.git"
	rewritten := detector.ensureScheme(scp)
	if !strings.HasPrefix(rewritten, "ssh://") {
		t.Errorf("Expected rewritten SCP-style URL to use ssh://, got %s", rewritten)
	}
	plain := "example.com/repo.git"
	defaulted := detector.ensureScheme(plain)
	if !strings.HasPrefix(defaulted, "https://") {
		t.Errorf("Expected default scheme https://, got %s", defaulted)
	}
}

func TestNormalizePath(t *testing.T) {
	detector := &CustomGitDetector{}
	uObj, err := url.Parse("https://example.com/some%20path")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}
	detector.normalizePath(uObj)
	if !strings.Contains(uObj.Path, " ") {
		t.Errorf("Expected normalized path to contain spaces, got %s", uObj.Path)
	}
}

func TestGetDefaultUsername(t *testing.T) {
	detector := CustomGitDetector{atmosConfig: &schema.AtmosConfiguration{}}
	if un := detector.getDefaultUsername(hostGitHub); un != "x-access-token" {
		t.Errorf("Expected x-access-token for GitHub, got %s", un)
	}
	if un := detector.getDefaultUsername(hostGitLab); un != "oauth2" {
		t.Errorf("Expected oauth2 for GitLab, got %s", un)
	}
	detector.atmosConfig.Settings.BitbucketUsername = "bbUser"
	if un := detector.getDefaultUsername(hostBitbucket); un != "bbUser" {
		t.Errorf("Expected bbUser for Bitbucket, got %s", un)
	}
	if un := detector.getDefaultUsername("unknown.com"); un != "x-access-token" {
		t.Errorf("Expected default x-access-token for unknown host, got %s", un)
	}
}

func TestDetect_DefaultsDepthToOneForShallowClone(t *testing.T) {
	// Supported git hosts get a shallow clone by default for speed.
	config := fakeAtmosConfig()
	detector := &CustomGitDetector{atmosConfig: &config, source: "repo.git"}
	result, ok, err := detector.Detect("github.com/cloudposse/atmos.git//examples", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !ok {
		t.Fatalf("Expected ok to be true for a supported host")
	}
	if !strings.Contains(result, "depth=1") {
		t.Errorf("Expected default shallow clone (depth=1), got: %s", result)
	}
}

// TestDetect_PreservesExplicitDepth pins a contract the scaffold catalog
// depends on (see templates.CatalogEntry.ResolvedSource): git rejects a
// shallow clone (`--depth`) combined with a ref that isn't a branch or tag,
// so pinning to an arbitrary commit SHA requires disabling the default
// shallow clone by passing `depth=0` explicitly. Detect must not override an
// explicit depth back to 1.
func TestDetect_PreservesExplicitDepth(t *testing.T) {
	config := fakeAtmosConfig()
	detector := &CustomGitDetector{atmosConfig: &config, source: "repo.git"}
	result, ok, err := detector.Detect("github.com/cloudposse/atmos.git//examples?ref=0cf62afa883b1546f07f2eaf2d6f1690353d31b7&depth=0", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !ok {
		t.Fatalf("Expected ok to be true for a supported host")
	}
	if !strings.Contains(result, "depth=0") {
		t.Errorf("Expected explicit depth=0 to be preserved, got: %s", result)
	}
	if strings.Contains(result, "depth=1") {
		t.Errorf("Expected depth not to be overridden to 1, got: %s", result)
	}
}

func TestDetect_UnsupportedHost(t *testing.T) {
	// This tests the branch when the URL host is not supported (not GitHub, GitLab, or Bitbucket)
	config := fakeAtmosConfig()
	detector := &CustomGitDetector{atmosConfig: &config, source: "repo.git"}
	unsupportedURL := "https://example.com/repo.git"
	result, ok, err := detector.Detect(unsupportedURL, "")
	if err != nil {
		t.Fatalf("Expected no error for unsupported host, got: %v", err)
	}
	if ok != false {
		t.Errorf("Expected ok to be false for unsupported host, got true")
	}
	if result != "" {
		t.Errorf("Expected result to be empty for unsupported host, got: %s", result)
	}
}

// Unix-specific test moved to custom_git_detector_unix_test.go:
// - TestRemoveSymlinks
