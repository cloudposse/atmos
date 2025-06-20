package telemetry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDocker(t *testing.T) {
	testCases := []struct {
		name          string
		cgroupContent string
		expected      bool
	}{
		{
			name:          "docker cgroup",
			cgroupContent: "0::/system.slice/docker.service",
			expected:      true,
		},
		{
			name:          "docker container cgroup",
			cgroupContent: "12:memory:/docker/1234567890abcdef",
			expected:      true,
		},
		{
			name:          "kubernetes cgroup",
			cgroupContent: "12:memory:/kubepods/pod12345678-1234-1234-1234-123456789012/container1234567890abcdef",
			expected:      true,
		},
		{
			name:          "host cgroup",
			cgroupContent: "0::/init.scope",
			expected:      false,
		},
		{
			name:          "regular cgroup",
			cgroupContent: "12:memory:/user.slice",
			expected:      false,
		},
		{
			name:          "empty content",
			cgroupContent: "",
			expected:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary file with the test content
			tmpfile, err := os.CreateTemp("", "cgroup_test")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())
			defer tmpfile.Close()

			if tc.cgroupContent != "" {
				if _, err := tmpfile.WriteString(tc.cgroupContent); err != nil {
					t.Fatal(err)
				}
			}

			// Test the cgroup detection logic with our test file
			result := isDocker(tmpfile.Name())
			assert.Equal(t, tc.expected, result)
		})
	}
}
