package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const defaultGoLicensesVersion = "v1.6.0"

// goInstallMaxAttempts and goInstallRetryDelay bound the retry loop around
// `go install github.com/google/go-licenses`: a transient mid-stream network
// error (e.g. a sum.golang.org HTTP/2 stream reset during go.sum
// verification) otherwise fails NOTICE generation outright on a single
// hiccup, unrelated to any real dependency problem. Matches the convention
// already used for `go mod download` (magefiles/build.go's
// runGoModDownload).
const (
	goInstallMaxAttempts = 3
	goInstallRetryDelay  = 15 * time.Second
)

// goInstallSleep is a package-level var so tests can swap in a no-op and
// exercise the retry loop without real 15s sleeps.
var goInstallSleep = time.Sleep

// runGoInstall runs `go install <module>`, streaming output directly to this
// process's stdout/stderr. A package-level var so tests can fake install
// failures/successes without a real subprocess or network access.
var runGoInstall = defaultRunGoInstall

func defaultRunGoInstall(module string) error {
	return runGoInstallWithTimeout(module, 3*time.Minute)
}

// Bound each attempt so a stalled download reaches the existing retry loop
// instead of consuming the entire CI job timeout.
func runGoInstallWithTimeout(module string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "install", module) // #nosec G204 -- module is a controlled default/env knob, not user input.
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("go install %s: %w", module, ctx.Err())
	}
	return err
}

// licenseEnv is the GOOS/GOARCH/CGO_ENABLED triple go-licenses and `go
// list` are run under, so license detection matches a specific build
// target rather than whatever platform happens to be running this tool.
type licenseEnv struct {
	GOOS       string
	GOARCH     string
	CGOEnabled string
}

func defaultLicenseEnv() licenseEnv {
	return licenseEnv{
		GOOS:       envOrDefault("LICENSE_GOOS", "linux"),
		GOARCH:     envOrDefault("LICENSE_GOARCH", "amd64"),
		CGOEnabled: envOrDefault("LICENSE_CGO_ENABLED", "1"),
	}
}

func (e licenseEnv) environ() []string {
	return append(os.Environ(), "GOOS="+e.GOOS, "GOARCH="+e.GOARCH, "CGO_ENABLED="+e.CGOEnabled)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func goLicensesVersion() string {
	return envOrDefault("GO_LICENSES_VERSION", defaultGoLicensesVersion)
}

// lookPathGoLicenses resolves an already-installed go-licenses binary on
// PATH. A package-level var so tests can force the "not found" branch
// deterministically instead of relying on the real PATH not already
// containing go-licenses (which it may, e.g. from a prior local run).
var lookPathGoLicenses = func() (string, error) {
	return exec.LookPath("go-licenses")
}

// ensureGoLicenses returns the path to a go-licenses binary, installing it
// via `go install` if it isn't already on PATH.
func ensureGoLicenses(version string) (string, error) {
	if path, err := lookPathGoLicenses(); err == nil {
		return path, nil
	}

	fmt.Fprintf(os.Stderr, "Installing go-licenses %s...\n", version)
	module := "github.com/google/go-licenses@" + version
	var lastErr error
	for attempt := 1; attempt <= goInstallMaxAttempts; attempt++ {
		lastErr = runGoInstall(module)
		if lastErr == nil {
			break
		}
		if attempt == goInstallMaxAttempts {
			return "", fmt.Errorf("go install go-licenses: failed after %d attempts: %w", goInstallMaxAttempts, lastErr)
		}
		fmt.Fprintf(os.Stderr, "go install go-licenses failed (attempt %d/%d), retrying in %s...\n",
			attempt, goInstallMaxAttempts, goInstallRetryDelay)
		goInstallSleep(goInstallRetryDelay)
	}

	binPath, err := resolveGoLicensesBinPath(goEnv)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(binPath); statErr != nil {
		return "", fmt.Errorf("go-licenses binary not found at %s: %w", binPath, statErr)
	}
	return binPath, nil
}

// resolveGoLicensesBinPath computes where `go install
// github.com/google/go-licenses` puts its binary: GOBIN if set, otherwise
// GOPATH/bin. goEnv is injected so this is testable without a real Go
// toolchain env lookup.
func resolveGoLicensesBinPath(goEnv func(key string) (string, error)) (string, error) {
	gobin, err := goEnv("GOBIN")
	if err != nil {
		return "", err
	}
	if gobin == "" {
		gopath, gopathErr := goEnv("GOPATH")
		if gopathErr != nil {
			return "", gopathErr
		}
		gobin = filepath.Join(gopath, "bin")
	}

	binPath := filepath.Join(gobin, "go-licenses")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	return binPath, nil
}

func goEnv(key string) (string, error) {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// noiseLinePrefix matches go-licenses' own warning/error log lines (e.g.
// "W1234: ..."), which it interleaves with its CSV report on stdout/stderr.
// Go module paths can't start with an uppercase letter, so this can't
// mistake a real CSV row for log noise.
var noiseLinePrefix = regexp.MustCompile(`^[WE]`)

// runLicenseReport runs `go-licenses report .` in root and parses its CSV
// output into entries. A nonzero exit with usable entries retains the existing
// partial-report behavior for unresolved licenses handled by URL overrides.
// Failed empty scans are retried and ultimately returned without writing NOTICE.
func runLicenseReport(binPath, root string, env licenseEnv) ([]LicenseEntry, error) {
	return collectLicenseReport(func() ([]byte, error) {
		cmd := exec.Command(binPath, "report", ".") // #nosec G204 -- binPath is resolved via exec.LookPath/go env, not user input.
		cmd.Dir = root
		cmd.Env = env.environ()

		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		return out.Bytes(), err
	}, licenseReportSleep)
}

const (
	licenseReportMaxAttempts = 3
	licenseReportRetryDelay  = 15 * time.Second
)

// licenseReportSleep lets subprocess tests exercise exhaustion without delays.
var licenseReportSleep = time.Sleep

// collectLicenseReport distinguishes an empty successful report from a failed
// scan. Keep scanner diagnostics so CI reports the actual failure, not a NOTICE
// diff deleting every dependency. The injected functions keep retries testable.
func collectLicenseReport(run func() ([]byte, error), sleep func(time.Duration)) ([]LicenseEntry, error) {
	for attempt := 1; ; attempt++ {
		output, err := run()
		entries := parseLicenseReport(string(output))
		if err == nil || len(entries) > 0 {
			return entries, nil
		}
		if attempt == licenseReportMaxAttempts {
			return nil, fmt.Errorf("go-licenses report failed without license entries after %d attempts: %w\n%s", attempt, err, output)
		}
		fmt.Fprintf(os.Stderr, "go-licenses report failed without license entries (attempt %d/%d): %v\n%s\nRetrying in %s...\n",
			attempt, licenseReportMaxAttempts, err, output, licenseReportRetryDelay)
		sleep(licenseReportRetryDelay)
	}
}

// parseLicenseReport parses go-licenses' `module,url,license` CSV output.
// go-licenses interleaves warning/error log lines (including multi-line
// ones whose continuation lines have no recognizable prefix) with its CSV
// rows on stdout/stderr, so each line is parsed independently and silently
// dropped if it isn't a clean 3-field CSV row -- matching the previous
// shell script's behavior, where such lines never matched any license
// family's grep pattern and so never reached NOTICE either.
func parseLicenseReport(raw string) []LicenseEntry {
	var entries []LicenseEntry
	for _, line := range strings.Split(raw, "\n") {
		if line == "" || noiseLinePrefix.MatchString(line) {
			continue
		}
		fields, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil || len(fields) != 3 {
			continue
		}
		entries = append(entries, LicenseEntry{Module: fields[0], URL: fields[1], License: fields[2]})
	}
	return entries
}
