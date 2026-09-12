// Package process measures resource usage — wall-clock time, CPU time, and
// (on platforms that support it) peak memory, page faults, context switches,
// and block I/O — for two distinct scopes: the current (atmos) process's own
// usage (via Baseline/Since/SelfUsageSoFar), and a subprocess tree's usage
// (via CollectFromProcessState, e.g. terraform/tofu plus its child provider
// plugins). Combine merges the two scopes together for callers that need a
// single number covering "everything this command did."
package process
