package container

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestIsAvailable_WithMocks tests isAvailable function with mocked executor.
func TestIsAvailable_WithMocks(t *testing.T) {
	tests := []struct {
		name            string
		runtimeType     Type
		setupMock       func(*MockCommandExecutor)
		expectAvailable bool
	}{
		{
			name:        "docker available - binary exists and info succeeds",
			runtimeType: TypeDocker,
			setupMock: func(m *MockCommandExecutor) {
				// LookPath succeeds.
				m.EXPECT().
					LookPath("docker").
					Return("/usr/bin/docker", nil).
					Times(1)

				// CommandContext returns a command that will succeed.
				cmd := exec.Command("true") // Use 'true' command which always succeeds.
				m.EXPECT().
					CommandContext(gomock.Any(), "docker", "info").
					Return(cmd).
					Times(1)
			},
			expectAvailable: true,
		},
		{
			name:        "docker unavailable - binary missing",
			runtimeType: TypeDocker,
			setupMock: func(m *MockCommandExecutor) {
				// LookPath fails.
				m.EXPECT().
					LookPath("docker").
					Return("", errors.New("executable file not found in $PATH")).
					Times(1)
			},
			expectAvailable: false,
		},
		{
			name:        "docker unavailable - binary exists but not running",
			runtimeType: TypeDocker,
			setupMock: func(m *MockCommandExecutor) {
				// LookPath succeeds.
				m.EXPECT().
					LookPath("docker").
					Return("/usr/bin/docker", nil).
					Times(1)

				// CommandContext returns a command that will fail.
				cmd := exec.Command("false") // Use 'false' command which always fails.
				m.EXPECT().
					CommandContext(gomock.Any(), "docker", "info").
					Return(cmd).
					Times(1)
			},
			expectAvailable: false,
		},
		{
			name:        "podman available - binary exists and info succeeds",
			runtimeType: TypePodman,
			setupMock: func(m *MockCommandExecutor) {
				// LookPath succeeds.
				m.EXPECT().
					LookPath("podman").
					Return("/usr/bin/podman", nil).
					Times(1)

				// CommandContext returns a command that will succeed.
				cmd := exec.Command("true")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "info").
					Return(cmd).
					Times(1)
			},
			expectAvailable: true,
		},
		{
			name:        "podman unavailable - binary missing",
			runtimeType: TypePodman,
			setupMock: func(m *MockCommandExecutor) {
				// LookPath fails.
				m.EXPECT().
					LookPath("podman").
					Return("", errors.New("executable file not found in $PATH")).
					Times(1)
			},
			expectAvailable: false,
		},
		{
			name:        "podman unavailable - binary exists but info fails (no auto-start)",
			runtimeType: TypePodman,
			setupMock: func(m *MockCommandExecutor) {
				// LookPath succeeds.
				m.EXPECT().
					LookPath("podman").
					Return("/usr/bin/podman", nil).
					Times(1)

				// First info command fails.
				cmdFail := exec.Command("false")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "info").
					Return(cmdFail).
					Times(1)

				// diagnoseUnresponsiveRuntime checks if machine exists.
				// Machine list command returns empty (no machines).
				cmdMachineList := exec.Command("echo", "")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
					Return(cmdMachineList).
					Times(1)

				// NOTE: No auto-start happens - isAvailable returns false without trying to init/start.
			},
			expectAvailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock controller.
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock executor.
			mockExec := NewMockCommandExecutor(ctrl)

			// Setup mock expectations.
			tt.setupMock(mockExec)

			// Inject mock executor.
			setExecutor(mockExec)
			defer resetExecutor()

			// Call the actual isAvailable function.
			ctx := context.Background()
			result := isAvailable(ctx, tt.runtimeType)

			// Assert result.
			assert.Equal(t, tt.expectAvailable, result)
		})
	}
}

// TestCheckRuntimeStatus tests the checkRuntimeStatus function.
func TestCheckRuntimeStatus(t *testing.T) {
	tests := []struct {
		name           string
		runtimeType    Type
		setupMock      func(*MockCommandExecutor)
		expectedStatus RuntimeStatus
	}{
		{
			name:        "podman needs init - no machine exists",
			runtimeType: TypePodman,
			setupMock: func(m *MockCommandExecutor) {
				m.EXPECT().
					LookPath("podman").
					Return("/usr/bin/podman", nil).
					Times(1)

				// Info fails.
				cmdInfoFail := exec.Command("false")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "info").
					Return(cmdInfoFail).
					Times(1)

				// Machine list returns empty.
				cmdMachineList := exec.Command("echo", "")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
					Return(cmdMachineList).
					Times(1)
			},
			expectedStatus: RuntimeNeedsInit,
		},
		{
			name:        "podman needs start - machine exists but not running",
			runtimeType: TypePodman,
			setupMock: func(m *MockCommandExecutor) {
				m.EXPECT().
					LookPath("podman").
					Return("/usr/bin/podman", nil).
					Times(1)

				// Info fails.
				cmdInfoFail := exec.Command("false")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "info").
					Return(cmdInfoFail).
					Times(1)

				// Machine list returns a machine.
				cmdMachineList := exec.Command("echo", "podman-machine-default")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
					Return(cmdMachineList).
					Times(1)
			},
			expectedStatus: RuntimeNeedsStart,
		},
		{
			name:        "docker not responding",
			runtimeType: TypeDocker,
			setupMock: func(m *MockCommandExecutor) {
				m.EXPECT().
					LookPath("docker").
					Return("/usr/bin/docker", nil).
					Times(1)

				// Info fails.
				cmdInfoFail := exec.Command("false")
				m.EXPECT().
					CommandContext(gomock.Any(), "docker", "info").
					Return(cmdInfoFail).
					Times(1)
			},
			expectedStatus: RuntimeNotResponding,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := NewMockCommandExecutor(ctrl)
			tt.setupMock(mockExec)

			setExecutor(mockExec)
			defer resetExecutor()

			ctx := context.Background()
			status := checkRuntimeStatus(ctx, tt.runtimeType)

			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}

// TestTryRecoverPodmanRuntime tests the explicit Podman recovery function.
func TestTryRecoverPodmanRuntime(t *testing.T) {
	t.Run("recovers podman - starts existing machine", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockExec := NewMockCommandExecutor(ctrl)

		// Initial status check.
		mockExec.EXPECT().
			LookPath("podman").
			Return("/usr/bin/podman", nil).
			Times(1)

		cmdInfoFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoFail).
			Times(1)

		// Machine exists.
		cmdMachineList := exec.Command("echo", "podman-machine-default")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
			Return(cmdMachineList).
			Times(1)

		// Start machine succeeds.
		cmdStart := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "start").
			Return(cmdStart).
			Times(1)

		// Verify info succeeds after start.
		cmdInfoSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoSuccess).
			Times(1)

		setExecutor(mockExec)
		defer resetExecutor()

		ctx := context.Background()
		status := TryRecoverPodmanRuntime(ctx)

		assert.Equal(t, RuntimeAvailable, status)
	})

	t.Run("recovers podman - initializes and starts new machine", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockExec := NewMockCommandExecutor(ctrl)

		// Initial status check.
		mockExec.EXPECT().
			LookPath("podman").
			Return("/usr/bin/podman", nil).
			Times(1)

		cmdInfoFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoFail).
			Times(1)

		// No machine exists.
		cmdMachineListEmpty := exec.Command("echo", "")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
			Return(cmdMachineListEmpty).
			Times(1)

		// Initialize machine.
		cmdInit := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "init").
			Return(cmdInit).
			Times(1)

		// Start machine.
		cmdStart := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "start").
			Return(cmdStart).
			Times(1)

		// Verify info succeeds.
		cmdInfoSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoSuccess).
			Times(1)

		setExecutor(mockExec)
		defer resetExecutor()

		ctx := context.Background()
		status := TryRecoverPodmanRuntime(ctx)

		assert.Equal(t, RuntimeAvailable, status)
	})

	t.Run("recovery fails - machine start fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockExec := NewMockCommandExecutor(ctrl)

		// Initial status check.
		mockExec.EXPECT().
			LookPath("podman").
			Return("/usr/bin/podman", nil).
			Times(1)

		cmdInfoFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoFail).
			Times(1)

		// Machine exists.
		cmdMachineList := exec.Command("echo", "podman-machine-default")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
			Return(cmdMachineList).
			Times(1)

		// Start machine fails.
		cmdStartFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "start").
			Return(cmdStartFail).
			Times(1)

		setExecutor(mockExec)
		defer resetExecutor()

		ctx := context.Background()
		status := TryRecoverPodmanRuntime(ctx)

		assert.Equal(t, RuntimeNeedsStart, status)
	})
}

// TestHasMachines tests the hasMachines helper function.
func TestHasMachines(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "single machine",
			output:   "podman-machine-default",
			expected: true,
		},
		{
			name: "multiple machines",
			output: `machine1
machine2
machine3`,
			expected: true,
		},
		{
			name:     "no machines - empty string",
			output:   "",
			expected: false,
		},
		{
			name:     "no machines - whitespace only",
			output:   "   \n\t  ",
			expected: false,
		},
		{
			name:     "machine with trailing newline",
			output:   "podman-machine-default\n",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasMachines(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRuntimeStatusMessage tests the user-friendly status messages.
func TestRuntimeStatusMessage(t *testing.T) {
	tests := []struct {
		name        string
		status      RuntimeStatus
		runtimeType Type
		contains    string
	}{
		{
			name:        "available",
			status:      RuntimeAvailable,
			runtimeType: TypeDocker,
			contains:    "available and running",
		},
		{
			name:        "unavailable",
			status:      RuntimeUnavailable,
			runtimeType: TypeDocker,
			contains:    "not installed",
		},
		{
			name:        "not responding",
			status:      RuntimeNotResponding,
			runtimeType: TypeDocker,
			contains:    "not responding",
		},
		{
			name:        "needs init",
			status:      RuntimeNeedsInit,
			runtimeType: TypePodman,
			contains:    "podman machine init",
		},
		{
			name:        "needs start",
			status:      RuntimeNeedsStart,
			runtimeType: TypePodman,
			contains:    "podman machine start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := RuntimeStatusMessage(tt.status, tt.runtimeType)
			assert.Contains(t, msg, tt.contains)
		})
	}
}
