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
			name:        "podman unavailable - binary exists but info fails and machine init fails",
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

				// tryStartPodmanMachine calls podmanMachineExists.
				// Machine list command returns empty (no machines).
				cmdMachineList := exec.Command("echo", "")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
					Return(cmdMachineList).
					Times(1)

				// Since no machine exists, tries to initialize.
				// Machine init fails.
				cmdInit := exec.Command("false")
				m.EXPECT().
					CommandContext(gomock.Any(), "podman", "machine", "init").
					Return(cmdInit).
					Times(1)
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

// TestIsAvailable_PodmanAutoStart tests Podman machine auto-start logic.
func TestIsAvailable_PodmanAutoStart(t *testing.T) {
	t.Run("podman auto-starts machine successfully", func(t *testing.T) {
		// Create mock controller.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mock executor.
		mockExec := NewMockCommandExecutor(ctrl)

		// Setup expectations.
		// LookPath succeeds.
		mockExec.EXPECT().
			LookPath("podman").
			Return("/usr/bin/podman", nil).
			Times(1)

		// First info command fails.
		cmdInfoFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoFail).
			Times(1)

		// tryStartPodmanMachine sequence:
		// 1. Check if machine exists.
		cmdMachineListSuccess := exec.Command("echo", "podman-machine-default")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
			Return(cmdMachineListSuccess).
			Times(1)

		// 2. Start machine (no init needed since machine exists).
		cmdStartSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "start").
			Return(cmdStartSuccess).
			Times(1)

		// 3. Retry info command - succeeds this time.
		cmdInfoSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoSuccess).
			Times(1)

		// Inject mock executor.
		setExecutor(mockExec)
		defer resetExecutor()

		// Call isAvailable.
		ctx := context.Background()
		result := isAvailable(ctx, TypePodman)

		// Should be available after auto-start.
		assert.True(t, result)
	})

	t.Run("podman auto-start fails - machine start fails", func(t *testing.T) {
		// Create mock controller.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mock executor.
		mockExec := NewMockCommandExecutor(ctrl)

		// Setup expectations.
		mockExec.EXPECT().
			LookPath("podman").
			Return("/usr/bin/podman", nil).
			Times(1)

		// First info command fails.
		cmdInfoFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoFail).
			Times(1)

		// tryStartPodmanMachine sequence:
		// 1. Check if machine exists.
		cmdMachineListSuccess := exec.Command("echo", "podman-machine-default")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
			Return(cmdMachineListSuccess).
			Times(1)

		// 2. Start machine fails.
		cmdStartFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "start").
			Return(cmdStartFail).
			Times(1)

		// Inject mock executor.
		setExecutor(mockExec)
		defer resetExecutor()

		// Call isAvailable.
		ctx := context.Background()
		result := isAvailable(ctx, TypePodman)

		// Should be unavailable - auto-start failed.
		assert.False(t, result)
	})

	t.Run("podman initializes and starts new machine", func(t *testing.T) {
		// Create mock controller.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create mock executor.
		mockExec := NewMockCommandExecutor(ctrl)

		// Setup expectations.
		mockExec.EXPECT().
			LookPath("podman").
			Return("/usr/bin/podman", nil).
			Times(1)

		// First info command fails.
		cmdInfoFail := exec.Command("false")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoFail).
			Times(1)

		// tryStartPodmanMachine sequence:
		// 1. Check if machine exists - none found.
		cmdMachineListEmpty := exec.Command("echo", "")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
			Return(cmdMachineListEmpty).
			Times(1)

		// 2. Initialize machine.
		cmdInitSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "init").
			Return(cmdInitSuccess).
			Times(1)

		// 3. Start machine.
		cmdStartSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "machine", "start").
			Return(cmdStartSuccess).
			Times(1)

		// 4. Retry info command - succeeds.
		cmdInfoSuccess := exec.Command("true")
		mockExec.EXPECT().
			CommandContext(gomock.Any(), "podman", "info").
			Return(cmdInfoSuccess).
			Times(1)

		// Inject mock executor.
		setExecutor(mockExec)
		defer resetExecutor()

		// Call isAvailable.
		ctx := context.Background()
		result := isAvailable(ctx, TypePodman)

		// Should be available after init + start.
		assert.True(t, result)
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
