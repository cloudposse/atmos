package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOIDCJWT_ForceTokenFile(t *testing.T) {
	tmp := t.TempDir()
	tokenPath := filepath.Join(tmp, "token.jwt")
	if err := os.WriteFile(tokenPath, []byte("abc"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	i := &awsOidc{ForceTokenFile: tokenPath}
	tok, err := i.loadOIDCJWT()
	if err != nil {
		t.Fatalf("loadOIDCJWT: %v", err)
	}
	if tok != "abc" {
		t.Fatalf("expected token 'abc', got %q", tok)
	}
}

func TestLoadOIDCJWT_NoSources(t *testing.T) {
	i := &awsOidc{}
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	if _, err := i.loadOIDCJWT(); err == nil {
		t.Fatalf("expected error when no token source")
	}
}

func TestFetchGitHubOIDCToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"tok"}`))
	}))
	defer srv.Close()
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "dummy")
	tok, err := fetchGitHubOIDCToken(context.Background(), "aud")
	if err != nil {
		t.Fatalf("fetchGitHubOIDCToken: %v", err)
	}
	if tok != "tok" {
		t.Fatalf("expected token 'tok', got %q", tok)
	}
}

func TestFetchGitHubOIDCToken_MissingEnv(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	if _, err := fetchGitHubOIDCToken(context.Background(), "aud"); err == nil {
		t.Fatalf("expected error with missing env vars")
	}
}
