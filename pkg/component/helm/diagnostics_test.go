package helm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ckerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	errUtils "github.com/cloudposse/atmos/errors"
)

// stubReleaseClientset makes newReleaseClientset return the given clientset (or
// error) for the duration of a test.
func stubReleaseClientset(t *testing.T, clientset kubernetes.Interface, err error) {
	t.Helper()
	original := newReleaseClientset
	t.Cleanup(func() { newReleaseClientset = original })
	newReleaseClientset = func(*actionContext) (kubernetes.Interface, error) {
		return clientset, err
	}
}

func crashLoopPod(name, namespace, release string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{releaseInstanceLabel: release},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 5m0s"},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"},
					},
				},
			},
		},
	}
}

func TestCollectReleaseFailureDiagnostics_CrashLoop(t *testing.T) {
	pod := crashLoopPod("keda-operator-7d9f", "keda", "keda")
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "ev1", Namespace: "keda"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "keda-operator-7d9f"},
		Reason:         "BackOff",
		Message:        "Back-off restarting failed container",
	}
	stubReleaseClientset(t, fake.NewSimpleClientset(pod, event), nil)

	spec := &chartSpec{ReleaseName: "keda", Namespace: "keda"}

	t.Run("non-verbose: status only", func(t *testing.T) {
		out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, spec, false)
		assert.Contains(t, out, "pod keda-operator-7d9f  app CrashLoopBackOff (exit 1, 5 restarts)")
		assert.NotContains(t, out, "last log")
		assert.NotContains(t, out, "events:")
	})

	t.Run("verbose: status + log tail + events", func(t *testing.T) {
		out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, spec, true)
		assert.Contains(t, out, "CrashLoopBackOff (exit 1, 5 restarts)")
		assert.Contains(t, out, "last log (app):")
		assert.Contains(t, out, "events:")
		assert.Contains(t, out, "BackOff  Back-off restarting failed container")
	})
}

func TestCollectReleaseFailureDiagnostics_SkipsHealthyAndEmpty(t *testing.T) {
	healthy := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "demo", Labels: map[string]string{releaseInstanceLabel: "demo"}},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "web", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	stubReleaseClientset(t, fake.NewSimpleClientset(healthy), nil)

	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "demo", Namespace: "demo"}, true)
	assert.Empty(t, out, "a healthy running pod produces no diagnostics")
}

func TestCollectReleaseFailureDiagnostics_NoPods(t *testing.T) {
	stubReleaseClientset(t, fake.NewSimpleClientset(), nil)
	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "x", Namespace: "x"}, true)
	assert.Empty(t, out)
}

func TestCollectReleaseFailureDiagnostics_ClientsetError(t *testing.T) {
	stubReleaseClientset(t, nil, errUtils.ErrHelmReleaseOperation)
	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "x", Namespace: "x"}, true)
	assert.Empty(t, out, "a clientset build error must not mask the original failure")
}

func TestCollectReleaseFailureDiagnostics_OnlyMatchingRelease(t *testing.T) {
	ours := crashLoopPod("ours", "ns", "mine")
	theirs := crashLoopPod("theirs", "ns", "other")
	stubReleaseClientset(t, fake.NewSimpleClientset(ours, theirs), nil)

	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "mine", Namespace: "ns"}, false)
	assert.Contains(t, out, "pod ours")
	assert.NotContains(t, out, "pod theirs", "only the release's own pods are reported")
}

func TestCollectReleaseFailureDiagnostics_PodListError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list pods failed")
	})
	stubReleaseClientset(t, cs, nil)

	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "r", Namespace: "ns"}, true)
	assert.Empty(t, out, "a pod-list error must not mask the original failure")
}

// TestCollectReleaseFailureDiagnostics_EventListErrorStillReportsStatus confirms a
// failure to list events (verbose mode) does not drop the container-status summary.
func TestCollectReleaseFailureDiagnostics_EventListErrorStillReportsStatus(t *testing.T) {
	cs := fake.NewSimpleClientset(crashLoopPod("p", "ns", "r"))
	cs.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list events failed")
	})
	stubReleaseClientset(t, cs, nil)

	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "r", Namespace: "ns"}, true)
	assert.Contains(t, out, "CrashLoopBackOff (exit 1, 5 restarts)")
	assert.NotContains(t, out, "events:", "an event-list error drops only the events section")
}

// TestPodEventLinesNewestFirstAndBounded covers the #3273 review: events are
// returned newest-first, bounded to diagnosticsMaxEvents, and scoped to the pod.
func TestPodEventLinesNewestFirstAndBounded(t *testing.T) {
	now := time.Now()
	var objs []runtime.Object
	for i := 0; i < 7; i++ {
		objs = append(objs, &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("e%d", i), Namespace: "ns"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "p"},
			Reason:         fmt.Sprintf("R%d", i),
			Message:        "msg",
			LastTimestamp:  metav1.NewTime(now.Add(time.Duration(i) * time.Minute)),
		})
	}
	// An event for a different pod must be excluded by the client-side check.
	objs = append(objs, &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "other", Namespace: "ns"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "q"},
		Reason:         "OTHER",
	})

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	lines := podEventLines(context.Background(), fake.NewSimpleClientset(objs...), pod)

	require.Len(t, lines, diagnosticsMaxEvents)
	assert.True(t, strings.HasPrefix(lines[0], "R6"), "newest event (R6) must be first, got %q", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "R5"))
	assert.NotContains(t, strings.Join(lines, "\n"), "OTHER", "another pod's events must be excluded")
}

// TestEventTimeFallbacks confirms the ordering timestamp falls back from
// LastTimestamp to EventTime to CreationTimestamp.
func TestEventTimeFallbacks(t *testing.T) {
	ts := time.Now().Truncate(time.Second)
	assert.Equal(t, ts, eventTime(&corev1.Event{LastTimestamp: metav1.NewTime(ts)}))
	assert.Equal(t, ts, eventTime(&corev1.Event{EventTime: metav1.NewMicroTime(ts)}))
	assert.Equal(t, ts, eventTime(&corev1.Event{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(ts)}}))
}

func TestContainerFailureSummary(t *testing.T) {
	exit := func(code int32, reason string) corev1.ContainerState {
		return corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: code, Reason: reason}}
	}
	tests := []struct {
		name    string
		status  corev1.ContainerStatus
		want    string
		failing bool
	}{
		{
			name:    "crashloop with last termination",
			status:  corev1.ContainerStatus{Name: "c", RestartCount: 3, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, LastTerminationState: exit(2, "Error")},
			want:    "c CrashLoopBackOff (exit 2, 3 restarts)",
			failing: true,
		},
		{
			name:    "image pull backoff with message",
			status:  corev1.ContainerStatus{Name: "c", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull access denied"}}},
			want:    "c ImagePullBackOff: pull access denied",
			failing: true,
		},
		{
			name:    "terminated nonzero",
			status:  corev1.ContainerStatus{Name: "c", RestartCount: 1, State: exit(137, "OOMKilled")},
			want:    "c OOMKilled (exit 137, 1 restarts)",
			failing: true,
		},
		{
			name:    "running is healthy",
			status:  corev1.ContainerStatus{Name: "c", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			failing: false,
		},
		{
			name:    "terminated zero is healthy",
			status:  corev1.ContainerStatus{Name: "c", State: exit(0, "Completed")},
			failing: false,
		},
		{
			name:    "transient waiting (ContainerCreating) is not a failure",
			status:  corev1.ContainerStatus{Name: "c", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}},
			failing: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			got, failing := containerFailureSummary(&status)
			assert.Equal(t, tt.failing, failing)
			if tt.failing {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestCollectReleaseFailureDiagnostics_InitContainer(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns", Labels: map[string]string{releaseInstanceLabel: "r"}},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{
				{Name: "init", RestartCount: 2, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}},
			},
		},
	}
	stubReleaseClientset(t, fake.NewSimpleClientset(pod), nil)
	out := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "r", Namespace: "ns"}, false)
	assert.Contains(t, out, "init CrashLoopBackOff (exit 1, 2 restarts)")
	assert.NotContains(t, out, "PodInitializing", "a transient init-wait on the app container is not reported")
}

func TestTruncateAndIndent(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 5))
	assert.Equal(t, "ab...", truncate("abcdef", 2))
	assert.Equal(t, "  a\n  b", indentLines("a\nb", "  "))
}

func TestIsFailureReason(t *testing.T) {
	for _, r := range []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "InvalidImageName"} {
		assert.True(t, isFailureReason(r), r)
	}
	for _, r := range []string{"ContainerCreating", "PodInitializing", "", "Running"} {
		assert.False(t, isFailureReason(r), r)
	}
}

// TestApplyReleaseFoldsPodDiagnosticsIntoUpgradeError is the end-to-end check: a
// failed upgrade collects crash-looping pod diagnostics and folds them into the
// returned release error (cloudposse/atmos#3271), before the rollback Atmos owns.
func TestApplyReleaseFoldsPodDiagnosticsIntoUpgradeError(t *testing.T) {
	actx := memoryActionContext(t)
	kubeClient := &failNextUpdateKubeClient{
		FailingKubeClient: actx.cfg.KubeClient.(*kubefake.FailingKubeClient),
	}
	actx.cfg.KubeClient = kubeClient
	stubActionContext(t, actx)
	stubReleaseClientset(t, fake.NewSimpleClientset(crashLoopPod("app-xyz", "testns", "diag-release")), nil)

	spec := testdataChartSpec(t, "diag-release")
	onFailure := string(failurePolicyRollback)
	spec.Release.Upgrade.OnFailure = &onFailure

	// Seed a successful install so the next apply takes the upgrade branch.
	_, err := applyRelease(context.Background(), spec, false)
	require.NoError(t, err)

	// The next upgrade fails; diagnostics must be folded into the error.
	kubeClient.failNext = true
	spec.Values["replicaCount"] = 99
	_, err = applyRelease(context.Background(), spec, false)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrHelmReleaseOperation)

	details := strings.Join(ckerrors.GetAllDetails(err), "\n")
	assert.Contains(t, details, "workload diagnostics:")
	assert.Contains(t, details, "app CrashLoopBackOff (exit 1, 5 restarts)")
}

// TestApplyReleaseFoldsPodDiagnosticsIntoInstallError covers the install path: a
// failed first install collects diagnostics and uninstalls (Atmos-owned) when the
// policy is uninstall-on-failure.
func TestApplyReleaseFoldsPodDiagnosticsIntoInstallError(t *testing.T) {
	actx := memoryActionContext(t)
	kubeClient := actx.cfg.KubeClient.(*kubefake.FailingKubeClient)
	kubeClient.WaitError = assertErr("install wait failed")
	stubActionContext(t, actx)
	stubReleaseClientset(t, fake.NewSimpleClientset(crashLoopPod("app-xyz", "testns", "install-diag")), nil)

	spec := testdataChartSpec(t, "install-diag")
	onFailure := string(failurePolicyUninstall)
	spec.Release.Install.OnFailure = &onFailure
	timeout := "5s"
	spec.Release.Install.Timeout = &timeout

	_, err := applyRelease(context.Background(), spec, false)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrHelmReleaseOperation)

	details := strings.Join(ckerrors.GetAllDetails(err), "\n")
	assert.Contains(t, details, "app CrashLoopBackOff (exit 1, 5 restarts)")

	// The uninstall-on-failure removed the failed release.
	_, getErr := getDeployedManifest(spec.ReleaseName, spec.Namespace)
	require.NoError(t, getErr)
}

// assertErr is a tiny error value for seeding a wait failure.
type assertErr string

func (e assertErr) Error() string { return string(e) }

// Guard that the diagnostics text would compose cleanly into an error explanation.
func TestDiagnosticsComposeIntoError(t *testing.T) {
	stubReleaseClientset(t, fake.NewSimpleClientset(crashLoopPod("p", "ns", "r")), nil)
	diag := collectReleaseFailureDiagnostics(context.Background(), &actionContext{}, &chartSpec{ReleaseName: "r", Namespace: "ns"}, false)
	require.NotEmpty(t, diag)
	assert.True(t, strings.HasPrefix(diag, "  pod p"))
}
