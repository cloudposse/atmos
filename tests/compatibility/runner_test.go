package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestComparisonPreservesTypesAndOrder(t *testing.T) {
	for _, pair := range [][2]string{{`true`, `1`}, {`1`, `1.0`}, {`1`, `"1"`}, {`null`, `""`}, {`[]`, `{}`}, {`[1,2]`, `[2,1]`}} {
		var a, b any
		if err := decode([]byte(pair[0]), &a); err != nil {
			t.Fatal(err)
		}
		if err := decode([]byte(pair[1]), &b); err != nil {
			t.Fatal(err)
		}
		if canonical(a) == canonical(b) {
			t.Errorf("coerced %s into %s", pair[0], pair[1])
		}
	}
	a := map[string]any{"a": json.Number("1"), "b": json.Number("2")}
	b := map[string]any{"b": json.Number("2"), "a": json.Number("1")}
	if canonical(a) != canonical(b) {
		t.Fatal("object key order affected comparison")
	}
}

func TestAllObservableBehaviorIsCompared(t *testing.T) {
	before := observation{Stdout: map[string]any{"text": "ok"}}
	changes := map[string]func(*observation){
		"exit":    func(o *observation) { o.ExitCode = 1 },
		"stdout":  func(o *observation) { o.Stdout = map[string]any{"text": "different"} },
		"warning": func(o *observation) { o.Stderr = "deprecated" },
		"request": func(o *observation) { o.Requests = []map[string]any{{"path": "/different"}} },
		"file":    func(o *observation) { o.Files = map[string]any{"output.tf": "changed"} },
		"notice":  func(o *observation) { o.FileNotices = []string{"unexpected"} },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			after := before
			change(&after)
			if !compare(io.Discard, map[string]observation{"case": before}, map[string]observation{"case": after}) {
				t.Fatal("difference accepted")
			}
		})
	}
	if !compare(io.Discard, map[string]observation{"missing": before}, map[string]observation{}) {
		t.Fatal("missing case accepted")
	}
	if !compare(io.Discard, map[string]observation{}, map[string]observation{"extra": before}) {
		t.Fatal("extra case accepted")
	}
}

func TestFileNoticesOnlyReorderKnownFiles(t *testing.T) {
	notices, stderr := fileNotices("✓ Created b.tf\nwarning\n✓ Created a.tf\n✓ Created unexpected.tf\n✓ Created a.tf\n", map[string]any{"a.tf": "", "b.tf": ""})
	if !reflect.DeepEqual(notices, []string{"✓ Created a.tf\n", "✓ Created a.tf\n", "✓ Created b.tf\n"}) {
		t.Fatal(notices)
	}
	if stderr != "warning\n✓ Created unexpected.tf\n" {
		t.Fatal(stderr)
	}
}

func TestNormalizationPreservesErrorDetails(t *testing.T) {
	sandbox := t.TempDir()
	before := fileURI(sandbox) + "/data.json\r\nhttp://127.0.0.1:12345/missing?x=1: 404"
	if got := normalize(before, sandbox, "http://127.0.0.1:12345"); got != "<SANDBOX>/data.json\n<HTTP>/missing?x=1: 404" {
		t.Fatal(got)
	}
}

func TestAmbientCredentialsAndConfigurationAreExcluded(t *testing.T) {
	for _, key := range []string{"AWS_PROFILE", "AWS_SECRET_ACCESS_KEY", "ATMOS_CONFIG", "HTTP_PROXY", "GITHUB_TOKEN"} {
		t.Setenv(key, "ambient-value")
	}
	env := environment("isolated-home", "isolated-work")
	for _, key := range []string{"AWS_PROFILE", "AWS_SECRET_ACCESS_KEY", "ATMOS_CONFIG", "HTTP_PROXY", "GITHUB_TOKEN"} {
		if _, ok := env[key]; ok {
			t.Errorf("inherited %s", key)
		}
	}
	if env["ATMOS_CLI_CONFIG_PATH"] != "isolated-work" {
		t.Fatal(env)
	}
}

func TestCorpusEditsInvalidateProvenance(t *testing.T) {
	root := t.TempDir()
	r := runner{root: root, here: filepath.Join(root, "runner"), fixtures: filepath.Join(root, "fixtures")}
	for _, dir := range []string{r.here, r.fixtures} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{filepath.Join(r.here, "cases.json"): "[]", filepath.Join(r.here, "go.mod"): "module test", filepath.Join(r.here, "main.go"): "package main", filepath.Join(r.fixtures, "data.json"): `{"count":1}`} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := r.fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.fixtures, "data.json"), []byte(`{"count":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := r.fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("fixture mutation did not invalidate baseline")
	}
}

func TestBaselineRejectsIncompleteObservations(t *testing.T) {
	r := runner{cases: []testCase{{ID: "ssm", Services: []string{"ssm"}}}}
	result := map[string]observation{"ssm": {}}
	if err := r.validateBaseline(result); err == nil || !strings.Contains(err.Error(), "never exercised") {
		t.Fatalf("got %v", err)
	}
	result["ssm"] = observation{Requests: []map[string]any{{"service": "ssm"}}}
	if err := r.validateBaseline(result); err != nil {
		t.Fatal(err)
	}
	result["ssm"] = observation{ExitCode: 1}
	if err := r.validateBaseline(result); err == nil {
		t.Fatal("unexpected old failure accepted")
	}
	result["ssm"] = observation{Files: map[string]any{"output.tf": nil}}
	if err := r.validateBaseline(result); err == nil {
		t.Fatal("missing old output accepted")
	}
}

func TestCandidateCannotRecordBaseline(t *testing.T) {
	for _, command := range []string{"compare", "verify"} {
		code, err := execute([]string{command, "--record", "--candidate", "candidate"})
		if code != 2 || err == nil || !strings.Contains(err.Error(), "only the pinned old") {
			t.Fatalf("candidate could record baseline: %d %v", code, err)
		}
	}
}
