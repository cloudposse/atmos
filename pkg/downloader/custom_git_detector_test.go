package downloader

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

func TestRemoveSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping symlink tests on Windows: symlinks require special privileges")
	}
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(tempDir, "link.txt")
	if err := os.Symlink(filePath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := removeSymlinks(tempDir); err != nil {
		t.Fatalf("removeSymlinks error: %v", err)
	}
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Errorf("Expected symlink to be removed, but it exists")
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("Expected regular file to exist, but got error: %v", err)
	}
}
