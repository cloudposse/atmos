// go_getter_utils_test.go
package exec

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-getter"

	"github.com/cloudposse/atmos/pkg/schema"
)

var originalDetectors = getter.Detectors

func fakeAtmosConfig(injectGit bool) schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			InjectGithubToken: injectGit,
		},
	}
}

// Test ValidateURI function.
func TestValidateURI(t *testing.T) {
	if err := ValidateURI(""); err == nil {
		t.Error("Expected error for empty URI, got nil")
	}
	uri := strings.Repeat("a", 2050)
	if err := ValidateURI(uri); err == nil {
		t.Error("Expected error for too-long URI, got nil")
	}
	if err := ValidateURI("http://example.com/../secret"); err == nil {
		t.Error("Expected error for path traversal sequence, got nil")
	}
	if err := ValidateURI("http://example.com/space test"); err == nil {
		t.Error("Expected error for spaces in URI, got nil")
	}
	if err := ValidateURI("http://example.com/path"); err != nil {
		t.Errorf("Expected valid URI, got error: %v", err)
	}
	if err := ValidateURI("oci://repo/path"); err != nil {
		t.Errorf("Expected valid OCI URI, got error: %v", err)
	}
	if err := ValidateURI("oci://repo"); err == nil {
		t.Error("Expected error for invalid OCI URI format, got nil")
	}
}

// Test IsValidScheme function.
func TestIsValidScheme(t *testing.T) {
	valid := []string{"http", "https", "git", "ssh", "git::https", "git::ssh"}
	for _, scheme := range valid {
		if !IsValidScheme(scheme) {
			t.Errorf("Expected scheme %s to be valid", scheme)
		}
	}
	if IsValidScheme("ftp") {
		t.Error("Expected scheme ftp to be invalid")
	}
}

// Test ensureScheme method.
func TestEnsureScheme(t *testing.T) {
	config := fakeAtmosConfig(false)
	detector := &CustomGitDetector{AtmosConfig: &config}
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

// Test rewriteSCPURL function.
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

// Test normalizePath method.
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

// Test injectToken method.
func TestInjectToken(t *testing.T) {
	os.Setenv("GITHUB_TOKEN", "testtoken")
	defer os.Unsetenv("GITHUB_TOKEN")
	config := fakeAtmosConfig(true)
	detector := &CustomGitDetector{AtmosConfig: &config}
	uObj, err := url.Parse("https://github.com/user/repo.git")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}
	detector.injectToken(uObj, hostGitHub)
	if uObj.User == nil {
		t.Error("Expected token to be injected into URL")
	} else {
		user := uObj.User.Username()
		if user != getDefaultUsername(hostGitHub) {
			t.Errorf("Expected username %s, got %s", getDefaultUsername(hostGitHub), user)
		}
	}
}

// Test resolveToken method.
func TestResolveToken(t *testing.T) {
	os.Setenv("GITHUB_TOKEN", "ghToken")
	defer os.Unsetenv("GITHUB_TOKEN")
	config := fakeAtmosConfig(true)
	detector := &CustomGitDetector{AtmosConfig: &config}
	token, source := detector.resolveToken(hostGitHub)
	if token != "ghToken" {
		t.Errorf("Expected token ghToken, got %s", token)
	}
	if source != "GITHUB_TOKEN" {
		t.Errorf("Unexpected token source: %s", source)
	}
}

// Test getDefaultUsername function.
func TestGetDefaultUsername(t *testing.T) {
	if un := getDefaultUsername(hostGitHub); un != "x-access-token" {
		t.Errorf("Expected x-access-token for GitHub, got %s", un)
	}
	if un := getDefaultUsername(hostGitLab); un != "oauth2" {
		t.Errorf("Expected oauth2 for GitLab, got %s", un)
	}
	os.Setenv("BITBUCKET_USERNAME", "bbUser")
	defer os.Unsetenv("BITBUCKET_USERNAME")
	if un := getDefaultUsername(hostBitbucket); un != "bbUser" {
		t.Errorf("Expected bbUser for Bitbucket, got %s", un)
	}
	if un := getDefaultUsername("unknown.com"); un != "x-access-token" {
		t.Errorf("Expected default x-access-token for unknown host, got %s", un)
	}
}

// Test adjustSubdir method.
func TestAdjustSubdir(t *testing.T) {
	detector := &CustomGitDetector{}
	uObj, err := url.Parse("https://github.com/user/repo.git")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}
	source := "repo.git"
	detector.adjustSubdir(uObj, source)
	if !strings.Contains(uObj.Path, "//.") {
		t.Errorf("Expected '//.' appended to path, got %s", uObj.Path)
	}
	uObj2, err := url.Parse("https://github.com/user/repo.git//subdir")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}
	detector.adjustSubdir(uObj2, "repo.git//subdir")
	if strings.HasSuffix(uObj2.Path, "//.") {
		t.Errorf("Did not expect subdir adjustment, got %s", uObj2.Path)
	}
}

// Test removeSymlinks function.
func TestRemoveSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink tests on Windows.")
	}
	tempDir, err := os.MkdirTemp("", "symlinktest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
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

// Test GoGetterGet using file scheme.
func TestGoGetterGet_File(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping file copying test on Windows due to potential file system differences.")
	}
	srcDir, err := os.MkdirTemp("", "src")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)
	srcFile := filepath.Join(srcDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(srcFile, content, 0o600); err != nil {
		t.Fatal(err)
	}
	// Create a temporary directory for destination and specify a destination file path.
	destDir, err := os.MkdirTemp("", "dest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(destDir)
	destFile := filepath.Join(destDir, "downloaded.txt")
	srcURL := "file://" + srcFile
	config := fakeAtmosConfig(false)
	err = GoGetterGet(&config, srcURL, destFile, getter.ClientModeFile, 5*time.Second)
	if err != nil {
		t.Errorf("GoGetterGet failed: %v", err)
	}
	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Errorf("Error reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("Expected file content %s, got %s", content, data)
	}
}

// Test DownloadDetectFormatAndParseFile.
func TestDownloadDetectFormatAndParseFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "detectparse")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.json")
	jsonContent := []byte(`{"key": "value"}`)
	if err := os.WriteFile(testFile, jsonContent, 0o600); err != nil {
		t.Fatal(err)
	}
	config := fakeAtmosConfig(false)
	result, err := DownloadDetectFormatAndParseFile(&config, "file://"+testFile)
	if err != nil {
		t.Errorf("DownloadDetectFormatAndParseFile error: %v", err)
	}
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Errorf("Expected result to be a map, got %T", result)
	} else if resMap["key"] != "value" {
		t.Errorf("Expected key to be 'value', got %v", resMap["key"])
	}
}

// Test RegisterCustomDetectors.
func TestRegisterCustomDetectors(t *testing.T) {
	orig := getter.Detectors
	getter.Detectors = []getter.Detector{}
	defer func() { getter.Detectors = orig }()
	config := fakeAtmosConfig(false)
	RegisterCustomDetectors(&config)
	if len(getter.Detectors) == 0 {
		t.Error("Expected at least one detector after registration.")
	}
	if _, ok := getter.Detectors[0].(*CustomGitDetector); !ok {
		t.Error("Expected first detector to be CustomGitDetector.")
	}
}

// Additional test for ValidateURI error paths.
func TestValidateURI_ErrorPaths(t *testing.T) {
	err := ValidateURI("http://example.com/with space")
	if err == nil {
		t.Error("Expected error for URI with space")
	}
	err = ValidateURI("http://example.com/../secret")
	if err == nil {
		t.Error("Expected error for URI with path traversal")
	}
}

// Additional test for rewriteSCPURL with no match.
func TestRewriteSCPURL_NoMatch(t *testing.T) {
	nonSCP := "not-an-scp-url"
	_, rewritten := rewriteSCPURL(nonSCP)
	if rewritten {
		t.Error("Expected non-SCP URL not to be rewritten")
	}
}

// Additional test for normalizePath error handling.
// Create a URL manually with an invalid escape sequence in the Path.
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

func TestMain(m *testing.M) {
	code := m.Run()
	getter.Detectors = originalDetectors
	os.Exit(code)
}
