package helm

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The releaseInstanceLabel constant is the standard Helm label applied to every
	// resource in a release; it is the canonical selector for a release's pods.
	releaseInstanceLabel = "app.kubernetes.io/instance"
	// The diagnosticsMaxPods constant bounds how many not-ready pods are reported so
	// a wide failure (e.g. a whole DaemonSet) does not produce an unbounded error.
	diagnosticsMaxPods = 10
	// The diagnosticsMaxEvents constant bounds the events reported per pod.
	diagnosticsMaxEvents = 5
	// The diagnosticsLogTailLines constant is how many log lines to tail from a
	// failing container when verbose diagnostics are enabled.
	diagnosticsLogTailLines = 20
	// The diagnosticsMaxMessageLen constant truncates long container and event
	// messages so one noisy message cannot dominate the error.
	diagnosticsMaxMessageLen = 200
	// The diagNewline constant is the line separator used throughout the formatted
	// diagnostics output.
	diagNewline = "\n"
)

// newReleaseClientset builds a Kubernetes clientset for the release's cluster from
// the Helm action settings. It is a package variable so tests can inject a fake
// clientset without a live cluster.
var newReleaseClientset = func(actx *actionContext) (kubernetes.Interface, error) {
	restConfig, err := actx.settings.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restConfig)
}

// collectReleaseFailureDiagnostics inspects the release's pods after a readiness
// failure and returns a human-readable summary of not-ready containers (reason,
// exit code, restart count). When verbose, it also appends a short log tail from
// each failing container and that pod's recent events.
//
// Diagnostics are strictly best-effort: any cluster-access error returns an empty
// string (logged at debug) so the collector never masks or replaces the original
// release failure. It is meant to run BEFORE a rollback/uninstall deletes the
// failing pods (cloudposse/atmos#3271).
func collectReleaseFailureDiagnostics(ctx context.Context, actx *actionContext, spec *chartSpec, verbose bool) string {
	defer perf.Track(nil, "helm.collectReleaseFailureDiagnostics")()

	clientset, err := newReleaseClientset(actx)
	if err != nil {
		log.Debug("helm: could not build clientset for failure diagnostics", "error", err)
		return ""
	}

	pods, err := clientset.CoreV1().Pods(spec.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: releaseInstanceLabel + "=" + spec.ReleaseName,
	})
	if err != nil {
		log.Debug("helm: could not list pods for failure diagnostics", "error", err)
		return ""
	}

	var b strings.Builder
	reported := 0
	for i := range pods.Items {
		if reported >= diagnosticsMaxPods {
			break
		}
		section := describeNotReadyPod(ctx, clientset, &pods.Items[i], verbose)
		if section == "" {
			continue
		}
		b.WriteString(section)
		reported++
	}
	return strings.TrimRight(b.String(), diagNewline)
}

// describeNotReadyPod returns the diagnostic lines for a single pod's not-ready
// containers, or "" when every container is healthy. Verbose mode adds a log tail
// per failing container and the pod's recent events.
func describeNotReadyPod(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod, verbose bool) string {
	statuses := allContainerStatuses(pod)
	var failing []*corev1.ContainerStatus
	var lines []string
	for i := range statuses {
		status := &statuses[i]
		summary, isFailing := containerFailureSummary(status)
		if !isFailing {
			continue
		}
		lines = append(lines, fmt.Sprintf("  pod %s  %s", pod.Name, summary))
		failing = append(failing, status)
	}
	if len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(strings.Join(lines, diagNewline))
	b.WriteString(diagNewline)

	if verbose {
		writeVerboseDiagnostics(ctx, &b, clientset, pod, failing)
	}
	return b.String()
}

// writeVerboseDiagnostics appends the per-container log tail and the pod's recent
// events, used only at debug/trace level.
func writeVerboseDiagnostics(ctx context.Context, b *strings.Builder, clientset kubernetes.Interface, pod *corev1.Pod, failing []*corev1.ContainerStatus) {
	for _, status := range failing {
		if tail := podLogTail(ctx, clientset, pod, status); tail != "" {
			fmt.Fprintf(b, "    last log (%s):%s%s%s", status.Name, diagNewline, indentLines(tail, "      "), diagNewline)
		}
	}
	events := podEventLines(ctx, clientset, pod)
	if len(events) == 0 {
		return
	}
	b.WriteString("    events:")
	b.WriteString(diagNewline)
	for _, event := range events {
		b.WriteString("      " + event + diagNewline)
	}
}

// allContainerStatuses returns init + regular container statuses so a failing init
// container (a common cause of a stuck rollout) is reported too.
func allContainerStatuses(pod *corev1.Pod) []corev1.ContainerStatus {
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	return statuses
}

// containerFailureSummary returns a one-line summary and whether the container is
// in a failing state. A container is failing when it is waiting on a non-transient
// reason (CrashLoopBackOff, ImagePullBackOff, ...) or has terminated with a
// non-zero exit code. A running or successfully-completed container is not failing.
func containerFailureSummary(status *corev1.ContainerStatus) (string, bool) {
	switch {
	case status.State.Waiting != nil && isFailureReason(status.State.Waiting.Reason):
		return waitingSummary(status), true
	case status.State.Terminated != nil && status.State.Terminated.ExitCode != 0:
		return terminatedSummary(status.Name, status.State.Terminated, status.RestartCount), true
	case status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 && status.RestartCount > 0:
		// A CrashLoopBackOff container's current state is Waiting; its crash detail
		// lives in the last-termination state.
		return terminatedSummary(status.Name, status.LastTerminationState.Terminated, status.RestartCount), true
	default:
		return "", false
	}
}

// waitingSummary summarizes a container stuck in a failing Waiting state.
func waitingSummary(status *corev1.ContainerStatus) string {
	reason := status.State.Waiting.Reason
	if last := status.LastTerminationState.Terminated; last != nil {
		return fmt.Sprintf("%s %s (exit %d, %d restarts)", status.Name, reason, last.ExitCode, status.RestartCount)
	}
	msg := strings.TrimSpace(status.State.Waiting.Message)
	if msg != "" {
		return fmt.Sprintf("%s %s: %s", status.Name, reason, truncate(msg, diagnosticsMaxMessageLen))
	}
	return fmt.Sprintf("%s %s", status.Name, reason)
}

// terminatedSummary summarizes a container that exited with a non-zero code.
func terminatedSummary(name string, term *corev1.ContainerStateTerminated, restarts int32) string {
	reason := strings.TrimSpace(term.Reason)
	if reason == "" {
		reason = "Terminated"
	}
	return fmt.Sprintf("%s %s (exit %d, %d restarts)", name, reason, term.ExitCode, restarts)
}

// isFailureReason reports whether a container Waiting reason indicates a real
// failure rather than a transient, expected startup state.
func isFailureReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff",
		"ImagePullBackOff",
		"ErrImagePull",
		"CreateContainerConfigError",
		"CreateContainerError",
		"InvalidImageName",
		"RunContainerError":
		return true
	default:
		return false
	}
}

// podLogTail returns the last lines of the failing container's log. For a
// restarted (crash-looping) container it reads the previous instance's log, which
// holds the crash output; otherwise it reads the current instance.
func podLogTail(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod, status *corev1.ContainerStatus) string {
	tail := int64(diagnosticsLogTailLines)
	opts := &corev1.PodLogOptions{
		Container: status.Name,
		TailLines: &tail,
		Previous:  status.RestartCount > 0,
	}
	raw, err := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts).DoRaw(ctx)
	if err != nil {
		log.Debug("helm: could not read pod log for failure diagnostics", "pod", pod.Name, "container", status.Name, "error", err)
		return ""
	}
	return strings.TrimRight(string(raw), diagNewline)
}

// podEventLines returns up to diagnosticsMaxEvents recent events for the pod,
// formatted "Reason  Message".
func podEventLines(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod) []string {
	events, err := clientset.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Debug("helm: could not list events for failure diagnostics", "pod", pod.Name, "error", err)
		return nil
	}
	var lines []string
	for i := range events.Items {
		event := &events.Items[i]
		if event.InvolvedObject.Kind != "Pod" || event.InvolvedObject.Name != pod.Name {
			continue
		}
		message := strings.TrimSpace(event.Message)
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("%s  %s", event.Reason, truncate(message, diagnosticsMaxMessageLen))))
		if len(lines) >= diagnosticsMaxEvents {
			break
		}
	}
	return lines
}

// indentLines prefixes every line of s with prefix.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, diagNewline)
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, diagNewline)
}

// truncate shortens s to maxLen bytes, appending an ellipsis when it was cut.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
