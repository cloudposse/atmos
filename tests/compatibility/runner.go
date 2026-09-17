package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture symlink is not allowed: %s", path)
		}
		sourceFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer sourceFile.Close()
		destFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(destFile, sourceFile)
		closeErr := destFile.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func environment(home, work string) map[string]string {
	env := map[string]string{}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_CACHE_HOME": filepath.Join(home, "cache"),
		"ATMOS_CLI_CONFIG_PATH": work, "ATMOS_VERSION_CHECK_ENABLED": "false", "ATMOS_TELEMETRY_ENABLED": "false",
		"ATMOS_NO_COLOR": "true", "NO_COLOR": "1", "TERM": "dumb", "TZ": "UTC", "LANG": "C", "LC_ALL": "C", "CI": "true", "COLUMNS": "20000",
		"AWS_EC2_METADATA_DISABLED": "true", "COMPAT_VALUE": "fixture-env",
	} {
		env[key] = value
	}
	return env
}

func envList(env map[string]string) []string {
	keys := []string{}
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []string{}
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func substitute(c testCase, replacements map[string]string) (testCase, error) {
	text := canonical(c)
	for key, value := range replacements {
		quoted := strings.TrimSpace(canonical(value))
		text = strings.ReplaceAll(text, key, quoted[1:len(quoted)-1])
	}
	var replaced testCase
	err := decode([]byte(text), &replaced)
	return replaced, err
}

func (r *runner) prepare(c testCase, sandbox, endpoint string) (string, testCase, error) {
	if err := copyTree(r.fixtures, filepath.Join(sandbox, "fixtures")); err != nil {
		return "", c, err
	}
	data := filepath.Join(sandbox, "fixtures", "data")
	c, err := substitute(c, map[string]string{
		"@HTTP@": endpoint, "@AUTHORITY@": strings.TrimPrefix(endpoint, "http://"),
		"@DATA_URI@": fileURI(data), "@DATA_PATH@": filepath.ToSlash(data), "@SANDBOX@": sandbox,
	})
	if err != nil {
		return "", c, err
	}
	if c.Catalog != "" {
		return filepath.Join(sandbox, "fixtures", "catalogs", filepath.FromSlash(c.Catalog)), c, nil
	}
	work := filepath.Join(sandbox, "probe")
	for _, dir := range []string{"stacks", "components/terraform/probe"} {
		if err := os.MkdirAll(filepath.Join(work, filepath.FromSlash(dir)), 0o700); err != nil {
			return "", c, err
		}
	}
	if err := os.WriteFile(filepath.Join(work, "components", "terraform", "probe", "main.tf"), []byte("# No Terraform execution.\n"), 0o600); err != nil {
		return "", c, err
	}
	settings := map[string]any{"enabled": true, "evaluations": 1, "sprig": map[string]any{"enabled": true}, "gomplate": map[string]any{"enabled": true, "timeout": 5}}
	for key, value := range c.Settings {
		incoming, inMap := value.(map[string]any)
		existing, existingMap := settings[key].(map[string]any)
		if inMap && existingMap {
			for k, v := range incoming {
				existing[k] = v
			}
		} else {
			settings[key] = value
		}
	}
	config := map[string]any{
		"base_path": "./", "components": map[string]any{"terraform": map[string]any{"base_path": "components/terraform"}},
		"stacks":    map[string]any{"base_path": "stacks", "included_paths": []string{"dev"}, "name_template": "{{ .vars.stage }}"},
		"templates": map[string]any{"settings": settings}, "logs": map[string]any{"level": "Info"},
	}
	if err := writeJSON(filepath.Join(work, "atmos.yaml"), config); err != nil {
		return "", c, err
	}
	// Compact JSON flow mappings are valid YAML and need no YAML dependency.
	compact := func(value any) string {
		if value == nil {
			return "{}"
		}
		var b bytes.Buffer
		// canonical already guarantees valid JSON.
		_ = json.Compact(&b, []byte(canonical(value)))
		return b.String()
	}
	stackSettings := any(c.StackSettings)
	if len(c.StackSettings) == 0 {
		stackSettings = map[string]any{}
	}
	componentSettings := any(c.ComponentSettings)
	if len(c.ComponentSettings) == 0 {
		componentSettings = map[string]any{}
	}
	stack := "vars: {\"stage\":\"dev\"}\nsettings: " + compact(stackSettings) + "\ncomponents:\n  terraform:\n    probe:\n      settings: " + compact(componentSettings) + "\n      vars:\n        result: |-\n          " + strings.ReplaceAll(c.Template, "\n", "\n          ") + "\n"
	if err := os.WriteFile(filepath.Join(work, "stacks", "dev.yaml"), []byte(stack), 0o600); err != nil {
		return "", c, err
	}
	format := c.Format
	if format == "" {
		format = "json"
	}
	c.Args = []string{"describe", "component", "probe", "-s", "dev", "--format", format, "--query", ".vars"}
	return work, c, nil
}

func normalize(text, sandbox, endpoint string) string {
	for _, source := range []string{fileURI(sandbox), sandbox, filepath.ToSlash(sandbox), strings.TrimLeft(filepath.ToSlash(sandbox), "/")} {
		text = strings.ReplaceAll(text, source, "<SANDBOX>")
	}
	authority := strings.TrimPrefix(endpoint, "http://")
	for _, pair := range [][2]string{{endpoint, "<HTTP>"}, {url.QueryEscape(endpoint), "<HTTP>"}, {url.QueryEscape(authority), "<AUTHORITY>"}, {authority, "<AUTHORITY>"}, {"\r\n", "\n"}} {
		text = strings.ReplaceAll(text, pair[0], pair[1])
	}
	return text
}

func fileNotices(stderr string, files map[string]any) ([]string, string) {
	known := map[string]bool{}
	for name := range files {
		known["✓ Created "+name+"\n"] = true
		known["✓ Created "+filepath.FromSlash(name)+"\n"] = true
	}
	notices := []string{}
	var remainder strings.Builder
	for _, line := range strings.SplitAfter(stderr, "\n") {
		if known[line] {
			notices = append(notices, line)
		} else {
			remainder.WriteString(line)
		}
	}
	sort.Strings(notices)
	return notices, remainder.String()
}

func hasService(c testCase, name string) bool {
	for _, s := range c.Services {
		if s == name {
			return true
		}
	}
	return false
}

func (r *runner) runCase(ctx context.Context, binary string, c testCase, server *fixtureServer) (observation, error) {
	o := observation{}
	sandbox, err := os.MkdirTemp("", "atmos-compat-case-")
	if err != nil {
		return o, err
	}
	defer os.RemoveAll(sandbox)
	sandbox, err = filepath.EvalSymlinks(sandbox)
	if err != nil {
		return o, err
	}
	work, materialized, err := r.prepare(c, sandbox, server.URL)
	if err != nil {
		return o, err
	}
	home := filepath.Join(sandbox, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		return o, err
	}
	env := environment(home, work)
	if hasService(c, "ssm") || hasService(c, "s3") {
		caFile := filepath.Join(home, "ca.pem")
		if err := os.WriteFile(caFile, server.caPEM, 0o600); err != nil {
			return o, err
		}
		for key, value := range map[string]string{
			"AWS_ACCESS_KEY_ID": accessKey, "AWS_SECRET_ACCESS_KEY": secretKey,
			"AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "us-east-1", "AWS_MAX_ATTEMPTS": "1", "AWS_SDK_LOAD_CONFIG": "1",
			"HTTPS_PROXY": server.URL, "HTTP_PROXY": server.URL, "NO_PROXY": "127.0.0.1,localhost", "AWS_CA_BUNDLE": caFile, "SSL_CERT_FILE": caFile,
		} {
			env[key] = value
		}
	}
	if hasService(c, "consul") {
		env["CONSUL_HTTP_TOKEN"] = "compat-token"
	}
	if hasService(c, "vault") {
		env["VAULT_TOKEN"] = "compat-vault-token"
		env["VAULT_ADDR"] = server.URL
	}
	for key, value := range materialized.Env {
		env[key] = value
	}
	server.reset()
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, binary, materialized.Args...)
	cmd.Dir = work
	cmd.Env = envList(env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if commandCtx.Err() != nil {
		return o, fmt.Errorf("case %s: %w", c.ID, commandCtx.Err())
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return o, err
		}
		o.ExitCode = exit.ExitCode()
	}
	output := normalize(stdout.String(), sandbox, server.URL)
	var value any
	if decode([]byte(output), &value) == nil {
		o.Stdout = map[string]any{"json": value}
	} else {
		o.Stdout = map[string]any{"text": output}
	}
	o.Files = map[string]any{}
	for _, name := range c.Files {
		data, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(name)))
		if errors.Is(err, os.ErrNotExist) {
			o.Files[name] = nil
		} else if err != nil {
			return o, err
		} else {
			o.Files[name] = normalize(string(data), sandbox, server.URL)
		}
	}
	o.FileNotices, o.Stderr = fileNotices(normalize(stderr.String(), sandbox, server.URL), o.Files)
	o.Requests = server.observations()
	for _, request := range o.Requests {
		if request["blocked_destination"] != nil || request["unsupported_operation"] != nil {
			return o, fmt.Errorf("fixture cannot serve %s: %s", c.ID, canonical(request))
		}
	}
	return o, nil
}

func (r *runner) runSuite(ctx context.Context, binary string) (map[string]observation, error) {
	server, err := newFixtureServer(r.fixtures)
	if err != nil {
		return nil, err
	}
	defer server.Close()
	results := map[string]observation{}
	for _, c := range r.cases {
		fmt.Fprintln(os.Stderr, "Running", c.ID)
		observation, err := r.runCase(ctx, binary, c, server)
		if err != nil {
			return nil, err
		}
		results[c.ID] = observation
	}
	return results, nil
}
