package testutils

import (
	"os"
	"testing"
)

func TestPreserveRestoreCIEnvVars(t *testing.T) {
	// Set a couple of CI environment variables
	os.Setenv("CI", "true")
	os.Setenv("GITHUB_ACTIONS", "true")

	env := PreserveCIEnvVars()
	if _, ok := os.LookupEnv("CI"); ok {
		t.Errorf("CI should be unset after PreserveCIEnvVars")
	}
	if _, ok := os.LookupEnv("GITHUB_ACTIONS"); ok {
		t.Errorf("GITHUB_ACTIONS should be unset after PreserveCIEnvVars")
	}
	if env["CI"] != "true" || env["GITHUB_ACTIONS"] != "true" {
		t.Errorf("preserved values incorrect: %+v", env)
	}

	RestoreCIEnvVars(env)
	if v := os.Getenv("CI"); v != "true" {
		t.Errorf("CI not restored, got %s", v)
	}
	if v := os.Getenv("GITHUB_ACTIONS"); v != "true" {
		t.Errorf("GITHUB_ACTIONS not restored, got %s", v)
	}
}

func TestPreserveCIEnvVarsNoVars(t *testing.T) {
	os.Unsetenv("CI")
	os.Unsetenv("GITHUB_ACTIONS")

	env := PreserveCIEnvVars()
	if len(env) != 0 {
		t.Errorf("expected no env vars preserved, got %d", len(env))
	}
}
