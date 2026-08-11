// Package process measures the current (atmos) process's own resource usage —
// wall-clock time, CPU time, and (on platforms that support it) peak memory,
// page faults, context switches, and block I/O. It measures the atmos process
// itself, not any terraform/tofu subprocess it may shell out to.
package process
