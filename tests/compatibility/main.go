// Package main compares actual Atmos CLIs against a pinned pre-migration build.
// This separate module intentionally depends only on the Go standard library.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const revision = "4ffd8804d3500b4369e739095174000565b43973"

type testCase struct {
	ID                string            `json:"id"`
	Template          string            `json:"template"`
	Catalog           string            `json:"catalog"`
	Args              []string          `json:"args"`
	Settings          map[string]any    `json:"settings"`
	StackSettings     map[string]any    `json:"stack_settings"`
	ComponentSettings map[string]any    `json:"component_settings"`
	Env               map[string]string `json:"env"`
	Services          []string          `json:"services"`
	Files             []string          `json:"files"`
	Format            string            `json:"format"`
	BaselineExit      int               `json:"baseline_exit"`
}

type observation struct {
	ExitCode    int              `json:"exit_code"`
	Stdout      map[string]any   `json:"stdout"`
	Stderr      string           `json:"stderr"`
	Requests    []map[string]any `json:"requests"`
	Files       map[string]any   `json:"files"`
	FileNotices []string         `json:"file_notices"`
}

type snapshot struct {
	Revision     string                 `json:"revision"`
	CorpusSHA256 string                 `json:"corpus_sha256"`
	GoVersion    string                 `json:"go_version"`
	Platform     string                 `json:"platform"`
	Cases        map[string]observation `json:"cases"`
}

type runner struct {
	root, here, fixtures string
	cases                []testCase
}

// canonical preserves JSON number spellings, scalar types and array order.
func canonical(value any) string {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		panic(err)
	}
	return b.String()
}

func decode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON data: %v", err)
	}
	return nil
}

func loadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decode(data, value)
}

func writeJSON(path string, value any) error {
	return os.WriteFile(path, []byte(canonical(value)), 0o600)
}

func newRunner(root string) (*runner, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r := &runner{root: root, here: filepath.Join(root, "tests", "compatibility"), fixtures: filepath.Join(root, "tests", "fixtures", "compatibility")}
	if err := loadJSON(filepath.Join(r.here, "cases.json"), &r.cases); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, c := range r.cases {
		if c.ID == "" || seen[c.ID] {
			return nil, fmt.Errorf("empty or duplicate case ID %q", c.ID)
		}
		seen[c.ID] = true
	}
	if len(r.cases) == 0 {
		return nil, errors.New("empty compatibility corpus")
	}
	return r, nil
}

func (r *runner) fingerprint() (string, error) {
	paths := []string{filepath.Join(r.here, "cases.json"), filepath.Join(r.here, "go.mod")}
	sources, err := filepath.Glob(filepath.Join(r.here, "*.go"))
	if err != nil {
		return "", err
	}
	for _, path := range sources {
		if !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
	}
	err = filepath.WalkDir(r.fixtures, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		name, err := filepath.Rel(r.root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(name) + "\x00"))
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (r *runner) validateBaseline(results map[string]observation) error {
	if len(results) != len(r.cases) {
		return errors.New("baseline does not contain the complete corpus")
	}
	for _, c := range r.cases {
		o, ok := results[c.ID]
		if !ok {
			return fmt.Errorf("missing baseline case %s", c.ID)
		}
		if o.ExitCode != c.BaselineExit {
			return fmt.Errorf("unexpected baseline exit for %s: %s", c.ID, canonical(o))
		}
		for name, value := range o.Files {
			if value == nil {
				return fmt.Errorf("missing generated baseline file %s for %s", name, c.ID)
			}
		}
		for _, service := range c.Services {
			found := false
			for _, req := range o.Requests {
				if req["service"] == service {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("baseline never exercised %s for %s", service, c.ID)
			}
		}
	}
	return nil
}

func (r *runner) buildBaseline(dir string) (string, error) {
	source := filepath.Join(dir, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		return "", err
	}
	archive, err := os.Create(filepath.Join(dir, "source.tar"))
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "archive", revision)
	cmd.Dir = r.root
	cmd.Stdout = archive
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	closeErr := archive.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	cmd = exec.Command("tar", "-xf", archive.Name(), "-C", source)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	binary := filepath.Join(dir, "atmos-old"+suffix)
	fmt.Fprintln(os.Stderr, "Building baseline", revision)
	cmd = exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binary, ".")
	cmd.Dir = source
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return binary, nil
}

func differences(expected, actual map[string]observation) []string {
	names := map[string]bool{}
	for name := range expected {
		names[name] = true
	}
	for name := range actual {
		names[name] = true
	}
	changed := []string{}
	for name := range names {
		a, aOK := expected[name]
		b, bOK := actual[name]
		if !aOK || !bOK || canonical(a) != canonical(b) {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

func compare(out io.Writer, expected, actual map[string]observation) bool {
	return compareDifferences(out, expected, actual, nil, differences(expected, actual))
}

func compareMigration(out io.Writer, expected, actual map[string]observation) bool {
	approved, changed := classifyDifferences(expected, actual)
	return compareDifferences(out, expected, actual, approved, changed)
}

func compareDifferences(out io.Writer, expected, actual map[string]observation, approved, changed []string) bool {
	for _, name := range approved {
		fmt.Fprintf(out, "APPROVED %s: BoltDB support intentionally removed; unsupported-scheme rejection verified.\n", name)
	}
	for _, name := range changed {
		fmt.Fprintf(out, "--- baseline/%s\n+++ candidate/%s\n", name, name)
		a, aOK := expected[name]
		b, bOK := actual[name]
		if !aOK || !bOK {
			fmt.Fprintf(out, "case present: old=%v new=%v\n", aOK, bOK)
			continue
		}
		// Print only changed fields; retain complete observations in the report.
		oldFields := map[string]any{}
		newFields := map[string]any{}
		_ = decode([]byte(canonical(a)), &oldFields)
		_ = decode([]byte(canonical(b)), &newFields)
		keys := []string{}
		for key := range oldFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if canonical(oldFields[key]) != canonical(newFields[key]) {
				fmt.Fprintf(out, "@@ %s @@\n- %s+ %s", key, canonical(oldFields[key]), canonical(newFields[key]))
			}
		}
	}
	matched := 0
	for name, a := range expected {
		if b, ok := actual[name]; ok && canonical(a) == canonical(b) {
			matched++
		}
	}
	fmt.Fprintf(out, "%d/%d cases match; %d approved differences; %d unapproved differences.\n", matched, len(expected), len(approved), len(changed))
	return len(changed) > 0
}

func binaryDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (r *runner) report(path, binary, digest string, expected, actual map[string]observation) error {
	if path == "" {
		return nil
	}
	binaryHash, err := binaryDigest(binary)
	if err != nil {
		return err
	}
	approved, unapproved := classifyDifferences(expected, actual)
	return writeJSON(path, map[string]any{
		"revision": revision, "corpus_sha256": digest, "platform": runtime.GOOS,
		"candidate_sha256": binaryHash, "baseline_cases": expected, "cases": actual,
		"approved_differences": approved, "unapproved_differences": unapproved,
	})
}

func execute(args []string) (int, error) {
	if len(args) == 0 {
		return 2, errors.New("usage: compatibility {baseline|compare|verify} [flags]")
	}
	command := args[0]
	if command != "baseline" && command != "compare" && command != "verify" {
		return 2, fmt.Errorf("unknown command %q", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	repo := flags.String("repo", "", "repository root (defaults to current Git worktree)")
	record := flags.Bool("record", false, "explicitly replace baseline from the pinned OLD source")
	candidate := flags.String("candidate", "", "path to candidate CLI")
	report := flags.String("report", "", "write old and candidate observations as JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return 2, err
	}
	if flags.NArg() != 0 {
		return 2, errors.New("unexpected positional arguments")
	}
	if *record && command != "baseline" {
		return 2, errors.New("only the pinned old implementation can record a baseline")
	}
	if command != "baseline" && *candidate == "" {
		return 2, errors.New("--candidate is required")
	}
	if command == "verify" && *report == "" {
		return 2, errors.New("--report is required for verify")
	}
	if *repo == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return 2, err
		}
		*repo = strings.TrimSpace(string(out))
	}
	r, err := newRunner(*repo)
	if err != nil {
		return 2, err
	}
	digest, err := r.fingerprint()
	if err != nil {
		return 2, err
	}
	baselinePath := filepath.Join(r.here, "baseline.json")
	expected := snapshot{}
	if !*record {
		if err := loadJSON(baselinePath, &expected); err != nil {
			return 2, err
		}
		if expected.Revision != revision || expected.CorpusSHA256 != digest {
			return 2, errors.New("baseline provenance/input mismatch; characterize the pinned OLD source again")
		}
	}
	ctx := context.Background()
	var results map[string]observation
	if command == "baseline" || command == "verify" {
		dir, err := os.MkdirTemp("", "atmos-compat-build-")
		if err != nil {
			return 2, err
		}
		defer os.RemoveAll(dir)
		binary, err := r.buildBaseline(dir)
		if err != nil {
			return 2, err
		}
		results, err = r.runSuite(ctx, binary)
		if err != nil {
			return 2, err
		}
		repeated, err := r.runSuite(ctx, binary)
		if err != nil {
			return 2, err
		}
		if compare(os.Stdout, results, repeated) {
			return 2, errors.New("old implementation is nondeterministic; baseline was not written")
		}
		if err := r.validateBaseline(results); err != nil {
			return 2, err
		}
		after, err := r.fingerprint()
		if err != nil {
			return 2, err
		}
		if after != digest {
			return 2, errors.New("inputs changed during collection")
		}
		captured := snapshot{revision, digest, runtime.Version(), runtime.GOOS, results}
		if *record {
			if err := writeJSON(baselinePath, captured); err != nil {
				return 2, err
			}
			fmt.Printf("Recorded %d cases from %s.\n", len(results), revision)
			return 0, nil
		}
		if command == "verify" {
			expected = captured
		} // Use a live old oracle on this OS.
	}
	if command != "baseline" {
		binary, err := filepath.Abs(*candidate)
		if err != nil {
			return 2, err
		}
		results, err = r.runSuite(ctx, binary)
		if err != nil {
			return 2, err
		}
		if err := r.report(*report, binary, digest, expected.Cases, results); err != nil {
			return 2, err
		}
	}
	after, err := r.fingerprint()
	if err != nil {
		return 2, err
	}
	if after != digest {
		return 2, errors.New("inputs changed during comparison")
	}
	comparison := compare
	if command != "baseline" {
		comparison = compareMigration
	}
	if comparison(os.Stdout, expected.Cases, results) {
		return 1, nil
	}
	return 0, nil
}

func main() {
	code, err := execute(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "compatibility harness error:", err)
	}
	os.Exit(code)
}
