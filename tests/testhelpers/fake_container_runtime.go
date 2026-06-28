package testhelpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FakeContainerRuntimeMode selects the behavior compiled into a fake docker or
// podman executable for tests that need to exercise CLI-backed runtime code.
type FakeContainerRuntimeMode string

const (
	FakeContainerRuntimeFull        FakeContainerRuntimeMode = "full"
	FakeContainerRuntimeStep        FakeContainerRuntimeMode = "step"
	FakeContainerRuntimeWorkflowEnv FakeContainerRuntimeMode = "workflow-env"
	FakeContainerRuntimeEmptyCreate FakeContainerRuntimeMode = "empty-create"
	FakeContainerRuntimePushError   FakeContainerRuntimeMode = "push-error"
)

// FakeContainerRuntimeSpec configures a fake container runtime executable.
type FakeContainerRuntimeSpec struct {
	Name string
	Mode FakeContainerRuntimeMode
}

const privateFileMode = 0o600

// InstallFakeContainerRuntime builds a tiny Go executable named after spec.Name
// and prepends it to PATH. The helper is intentionally a real executable so
// tests still exercise os/exec and PATH lookup without POSIX shell scripts.
func InstallFakeContainerRuntime(t *testing.T, spec FakeContainerRuntimeSpec) {
	t.Helper()

	require.NotEmpty(t, spec.Name)
	require.NotEmpty(t, spec.Mode)

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.go")
	binaryName := spec.Name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(dir, binaryName)

	require.NoError(t, os.WriteFile(sourcePath, []byte(fakeContainerRuntimeSource(spec)), privateFileMode))

	cmd := exec.Command("go", "build", "-o", binaryPath, sourcePath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build fake %s runtime failed:\n%s", spec.Name, string(output))

	t.Setenv("PATH", dir+string(os.PathListSeparator)+envValue("PATH"))
}

func fakeContainerRuntimeSource(spec FakeContainerRuntimeSpec) string {
	return fmt.Sprintf(fakeContainerRuntimeTemplate, spec.Name, spec.Mode)
}

func envValue(name string) string {
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == name {
			return value
		}
	}
	return ""
}

const fakeContainerRuntimeTemplate = `package main

import (
	"fmt"
	"os"
	"strings"
)

const runtimeName = %[1]q
const mode = %[2]q

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		return
	}

	if requiresForwardedEnv(args[0]) && os.Getenv("ATMOS_FAKE_AUTH") != "present" {
		fmt.Fprintln(os.Stderr, "missing forwarded env")
		os.Exit(9)
	}

	switch mode {
	case "full":
		fullRuntime(args)
	case "step":
		stepRuntime(args)
	case "workflow-env":
		workflowRuntime(args)
	case "empty-create":
		emptyCreateRuntime(args)
	case "push-error":
		pushErrorRuntime(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown fake runtime mode: %%s\n", mode)
		os.Exit(4)
	}
}

func requiresForwardedEnv(command string) bool {
	return (mode == "full" || mode == "workflow-env") && command != "info" && command != "version"
}

func fullRuntime(args []string) {
	switch args[0] {
	case "info":
		return
	case "version":
		fmt.Println("1.2.3")
	case "tag":
		fmt.Println("tagged")
	case "push":
		fmt.Println("latest: digest: sha256:abcdef1234567890 size: 1234")
	case "image":
		if len(args) > 1 && args[1] == "inspect" {
			fmt.Println(` + "`" + `{"Id":"sha256:abcdef","RepoTags":["app:local"],"RepoDigests":["app@sha256:abcdef"],"Size":2048,"Created":"2026-06-19T00:00:00Z","Architecture":"arm64","Os":"linux","Config":{"Labels":{"app":"test"}},"RootFS":{"Layers":["l1","l2"]}}` + "`" + `)
			return
		}
		fmt.Fprintln(os.Stderr, "unknown image command")
		os.Exit(4)
	case "create":
		fmt.Println("pull progress")
		fmt.Printf("%%s-container-id\n", runtimeName)
	case "start", "stop", "rm", "pull":
		return
	case "inspect":
		fmt.Println(` + "`" + `{"Id":"container-id","Name":"/box","Image":"sha256:img","State":{"Status":"running"},"Config":{"Labels":{"app":"test"}},"Created":"2026-06-19T00:00:00Z"}` + "`" + `)
	case "ps":
		if runtimeName == "podman" {
			fmt.Println(` + "`" + `[{"Id":"podman-id","Names":["podman-box"],"Image":"alpine","State":"running","Labels":{"app":"test"}}]` + "`" + `)
			return
		}
		fmt.Println(` + "`" + `{"ID":"docker-id","Names":"/docker-box","Image":"alpine","State":"running","Labels":"app=test"}` + "`" + `)
		fmt.Println("not-json")
	case "exec":
		fmt.Println("exec stdout")
	case "logs":
		fmt.Println("log stdout")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %%s\n", strings.Join(args, " "))
		os.Exit(4)
	}
}

func stepRuntime(args []string) {
	switch args[0] {
	case "info", "build", "pull", "start", "rm", "tag", "ps":
		return
	case "create":
		fmt.Println("container-id")
	case "exec":
		fmt.Println("run stdout")
	case "image":
		if len(args) > 1 && args[1] == "inspect" {
			fmt.Println(` + "`" + `{"Id":"sha256:built","RepoTags":["app:local"],"RepoDigests":["app@sha256:built"],"Size":1024,"Created":"2026-06-19T00:00:00Z","Architecture":"amd64","Os":"linux","Config":{"Labels":{"app":"test"}},"RootFS":{"Layers":["l1"]}}` + "`" + `)
			return
		}
		os.Exit(4)
	default:
		return
	}
}

func workflowRuntime(args []string) {
	switch args[0] {
	case "info", "start", "rm":
		return
	case "create":
		fmt.Println("container-id")
	case "exec":
		fmt.Println("container stdout")
	default:
		return
	}
}

func emptyCreateRuntime(args []string) {
	switch args[0] {
	case "info", "create":
		return
	default:
		return
	}
}

func pushErrorRuntime(args []string) {
	switch args[0] {
	case "info":
		return
	case "push":
		if runtimeName == "podman" {
			fmt.Println(` + "`" + `error\ndigest: sha256:feedface` + "`" + `)
		} else {
			fmt.Println("error digest: sha256:feedface")
		}
		os.Exit(7)
	default:
		return
	}
}
`
