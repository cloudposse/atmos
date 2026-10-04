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
	// releaseInstanceLabel is the standard Helm label applied to every resource in
	// a release; it is the canonical selector for finding a release's pods.
	releaseInstanceLabel = "app.kubernetes.io/instance"
	// diagnosticsMaxPods bounds how many not-ready pods are reported so a wide
	// failure (e.g. a whole DaemonSet) does not produce an unbounded error.
	diagnosticsMaxPods = 10
	// diagnosticsMaxEvents bounds the events reported per pod.
	diagnosticsMaxEvents = 5
	// diagnosticsLogTailLines is how many log lines to tail from a failing
	// container when verbose diagnostics are enabled.
	diagnosticsLogTailLines = 20
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
		pod := &pods.Items[i]
		section := describeNotReadyPod(ctx, clientset, pod, verbose)
		if section == "" {
			continue
		}
		b.WriteString(section)
		reported++
	}
	return strings.TrimRight(b.String(), "\n")
}

// describeNotReadyPod returns the diagnostic lines for a single pod's not-ready
// containers, or "" when every container is healthy. Verbose mode adds a log tail
// per failing container and the pod's recent events.
func describeNotReadyPod(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod, verbose bool) string {
	var failing []corev1.ContainerStatus
	var lines []string
	for _, status := range allContainerStatuses(pod) {
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
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")

	if verbose {
		for _, status := range failing {
			if tail := podLogTail(ctx, clientset, pod, status); tail != "" {
				b.WriteString(fmt.Sprintf("    last log (%s):\n%s\n", status.Name, indentLines(tail, "      ")))
			}
		}
		if events := podEventLines(ctx, clientset, pod); len(events) > 0 {
			b.WriteString("    events:\n")
			for _, e := range events {
				b.WriteString("      " + e + "\n")
			}
		}
	}
	return b.String()
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
func containerFailureSummary(status corev1.ContainerStatus) (string, bool) {
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

func waitingSummary(status corev1.ContainerStatus) string {
	reason := status.State.Waiting.Reason
	if last := status.LastTerminationState.Terminated; last != nil {
		return fmt.Sprintf("%s %s (exit %d, %d restarts)", status.Name, reason, last.ExitCode, status.RestartCount)
	}
	msg := strings.TrimSpace(status.State.Waiting.Message)
	if msg != "" {
		return fmt.Sprintf("%s %s: %s", status.Name, reason, truncate(msg, 200))
	}
	return fmt.Sprintf("%s %s", status.Name, reason)
}

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
		"RunContainerError",
		"CrashLoopBackoff":
		return true
	default:
		return false
	}
}

// podLogTail returns the last lines of the failing container's log. For a
// restarted (crash-looping) container it reads the previous instance's log, which
// holds the crash output; otherwise it reads the current instance.
func podLogTail(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod, status corev1.ContainerStatus) string {
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
	return strings.TrimRight(string(raw), "\n")
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
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("%s  %s", event.Reason, truncate(message, 200))))
		if len(lines) >= diagnosticsMaxEvents {
			break
		}
	}
	return lines
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
