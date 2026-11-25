package process

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/signals"
)

// Helper modes that stand in for Terraform reacting to an interrupt.
const (
	// Mode gracefulTrap records each SIGINT/SIGTERM, keeps "saving state" briefly after the
	// first one (recording any further signal), writes a done marker, and exits 0.
	modeGracefulTrap = "graceful-trap"
	// Mode gracefulFamily is gracefulTrap with a grandchild that records the signals it gets,
	// standing in for a provider plugin or a local-exec provisioner in Terraform's group.
	modeGracefulFamily     = "graceful-family"
	modeGracefulGrandchild = "graceful-grandchild"
	// Mode gracefulStuck records signals but never exits on its own, to exercise the forced path.
	modeGracefulStuck = "graceful-stuck"

	gracefulSaveTime = 300 * time.Millisecond
)

// runGracefulHelper implements the graceful helper modes.
func runGracefulHelper(mode, dir string) {
	received := make(chan os.Signal, 4)
	signal.Notify(received, os.Interrupt, syscall.SIGTERM)
	switch mode {
	case modeGracefulTrap, modeGracefulFamily:
		var grandchild *exec.Cmd
		if mode == modeGracefulFamily {
			grandchild = startGracefulGrandchild(dir)
		}
		writePIDFile(dir, "child")
		recordSignal(dir, "child", <-received)
		deadline := time.After(gracefulSaveTime)
		for saving := true; saving; {
			select {
			case sig := <-received:
				recordSignal(dir, "child", sig)
			case <-deadline:
				saving = false
			}
		}
		stopGracefulGrandchild(grandchild)
		_ = os.WriteFile(filepath.Join(dir, "done"), []byte("saved"), 0o600)
	case modeGracefulGrandchild, modeGracefulStuck:
		name := "child"
		if mode == modeGracefulGrandchild {
			name = "grandchild"
		}
		writePIDFile(dir, name)
		for sig := range received {
			recordSignal(dir, name, sig)
		}
	}
}

// startGracefulGrandchild starts the grandchild and waits until it can record signals.
func startGracefulGrandchild(dir string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	grandchild := exec.Command(exe)
	grandchild.Env = append(os.Environ(), helperModeEnv+"="+modeGracefulGrandchild)
	if err := grandchild.Start(); err != nil {
		os.Exit(2)
	}
	for readPID(dir, "grandchild") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	return grandchild
}

func stopGracefulGrandchild(grandchild *exec.Cmd) {
	if grandchild == nil {
		return
	}
	_ = grandchild.Process.Kill()
	_ = grandchild.Wait()
}

// recordSignal appends the received signal's name to <dir>/<name>.signals.
func recordSignal(dir, name string, sig os.Signal) {
	f, err := os.OpenFile(filepath.Join(dir, name+".signals"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(sig.String() + "\n")
	_ = f.Close()
}

// recordedSignals returns the signal names a helper recorded, in order.
func recordedSignals(dir, name string) []string {
	data, err := os.ReadFile(filepath.Join(dir, name+".signals"))
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

// useGracefulStops gives one test its own graceful-stop registry.
func useGracefulStops(t *testing.T) *signals.GracefulStops {
	t.Helper()
	previous := gracefulStops
	stops := signals.NewGracefulStops(nil, nil)
	gracefulStops = stops
	t.Cleanup(func() { gracefulStops = previous })
	return stops
}

// gracefulSpec builds a graceful TaskSpec that re-executes the test binary in mode.
func gracefulSpec(t *testing.T, mode, dir string) *TaskSpec {
	t.Helper()
	spec := helperSpec(t, mode, dir, nil)
	spec.Interrupt = InterruptGraceful
	return &spec
}

// beginOnceRegistered delivers the first signal as soon as the running child is registered.
func beginOnceRegistered(t *testing.T, stops *signals.GracefulStops, sig os.Signal) []string {
	t.Helper()
	var names []string
	require.Eventually(t, func() bool {
		var ok bool
		names, ok = stops.Begin(sig)
		return ok
	}, pidWait, 10*time.Millisecond, "the graceful child never joined the registry")
	return names
}
