package io

import (
	"bytes"
	stdio "io"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChildProcessOutput(t *testing.T) {
	for _, masked := range []bool{false, true} {
		config := &Config{DisableMasking: !masked}
		streams := newStreams(newMasker(config), config)
		assert.Same(t, os.Stdout, ChildProcessOutput(streams.Output()))
		assert.Same(t, os.Stderr, ChildProcessOutput(streams.Error()))
	}
	var captured bytes.Buffer
	writer := &dynamicWriter{getWriter: func() stdio.Writer { return &captured }}
	assert.Same(t, writer, ChildProcessOutput(writer))
	assert.Same(t, &captured, ChildProcessOutput(&captured))

	config := &Config{}
	streams := newStreams(newMasker(config), config)
	recorder := &testRecorder{}
	restore := SetRecorder(recorder)
	defer restore()
	assert.Same(t, streams.Output(), ChildProcessOutput(streams.Output()), "preserve active recording")
}

func TestChildProcessOutputPreservesTerminal(t *testing.T) {
	if os.Getenv("ATMOS_TEST_CHILD_TERMINAL") == "1" {
		if !term.IsTerminal(os.Stdout.Fd()) {
			os.Exit(1)
		}
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTYs are not supported on Windows")
	}
	master, slave, err := pty.Open()
	require.NoError(t, err)
	defer master.Close()
	defer slave.Close()
	config := &Config{}
	wrapped := &dynamicMaskedWriter{masker: newMasker(config), getWriter: func() stdio.Writer { return slave }}
	child := exec.Command(os.Args[0], "-test.run=^TestChildProcessOutputPreservesTerminal$")
	child.Env = append(os.Environ(), "ATMOS_TEST_CHILD_TERMINAL=1")
	child.Stdout = ChildProcessOutput(wrapped)
	child.Stderr = os.Stderr
	require.NoError(t, child.Run(), "child must see a terminal so its progress indicators remain enabled")
}
