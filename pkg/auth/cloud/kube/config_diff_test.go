package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// These tests exercise the no-op detection helpers' debug-diagnostic branches
// (gated by the debug bool / ATMOS_KUBECONFIG_DEBUG_DIFF env var). The
// diagnostics only emit a structured log line, so there's nothing to assert
// about the log output itself — the point is to drive execution through
// every debug branch (map length differs, key missing, entry differs, and
// the nested Exec-differs case) with debug both on and off, in addition to
// locking in the boolean contract of each comparison helper.

//nolint:dupl // Table structure mirrors TestContextMapsEqualDebug but exercises a distinct map type (Cluster vs Context).
func TestClusterMapsEqualDebug(t *testing.T) {
	clusterA := &clientcmdapi.Cluster{Server: "https://a"}
	clusterACopy := &clientcmdapi.Cluster{Server: "https://a"}
	clusterB := &clientcmdapi.Cluster{Server: "https://b"}

	tests := []struct {
		name  string
		a, b  map[string]*clientcmdapi.Cluster
		debug bool
		want  bool
	}{
		{
			name:  "equal maps",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k1": clusterACopy},
			debug: false,
			want:  true,
		},
		{
			name:  "length differs, debug off",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k1": clusterACopy, "k2": clusterB},
			debug: false,
			want:  false,
		},
		{
			name:  "length differs, debug on",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k1": clusterACopy, "k2": clusterB},
			debug: true,
			want:  false,
		},
		{
			name:  "key missing from b, debug off",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k2": clusterB},
			debug: false,
			want:  false,
		},
		{
			name:  "key missing from b, debug on",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k2": clusterB},
			debug: true,
			want:  false,
		},
		{
			name:  "entry differs, debug off",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k1": clusterB},
			debug: false,
			want:  false,
		},
		{
			name:  "entry differs, debug on",
			a:     map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:     map[string]*clientcmdapi.Cluster{"k1": clusterB},
			debug: true,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterMapsEqualDebug(tt.a, tt.b, tt.debug)
			assert.Equal(t, tt.want, got)
		})
	}
}

//nolint:dupl // Table structure mirrors TestClusterMapsEqualDebug but exercises a distinct map type (Context vs Cluster).
func TestContextMapsEqualDebug(t *testing.T) {
	ctxA := &clientcmdapi.Context{Cluster: "c1", AuthInfo: "a1"}
	ctxACopy := &clientcmdapi.Context{Cluster: "c1", AuthInfo: "a1"}
	ctxB := &clientcmdapi.Context{Cluster: "c2", AuthInfo: "a1"}

	tests := []struct {
		name  string
		a, b  map[string]*clientcmdapi.Context
		debug bool
		want  bool
	}{
		{
			name:  "equal maps",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k1": ctxACopy},
			debug: false,
			want:  true,
		},
		{
			name:  "length differs, debug off",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k1": ctxACopy, "k2": ctxB},
			debug: false,
			want:  false,
		},
		{
			name:  "length differs, debug on",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k1": ctxACopy, "k2": ctxB},
			debug: true,
			want:  false,
		},
		{
			name:  "key missing from b, debug off",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k2": ctxB},
			debug: false,
			want:  false,
		},
		{
			name:  "key missing from b, debug on",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k2": ctxB},
			debug: true,
			want:  false,
		},
		{
			name:  "entry differs, debug off",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k1": ctxB},
			debug: false,
			want:  false,
		},
		{
			name:  "entry differs, debug on",
			a:     map[string]*clientcmdapi.Context{"k1": ctxA},
			b:     map[string]*clientcmdapi.Context{"k1": ctxB},
			debug: true,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contextMapsEqualDebug(tt.a, tt.b, tt.debug)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuthInfoMapsEqualDebug(t *testing.T) {
	authA := &clientcmdapi.AuthInfo{Token: "t1"}
	authACopy := &clientcmdapi.AuthInfo{Token: "t1"}
	authB := &clientcmdapi.AuthInfo{Token: "t2"}
	authExecA := &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "atmos"}}
	authExecB := &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "other"}}

	tests := []struct {
		name  string
		a, b  map[string]*clientcmdapi.AuthInfo
		debug bool
		want  bool
	}{
		{
			name:  "equal maps",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k1": authACopy},
			debug: false,
			want:  true,
		},
		{
			name:  "length differs, debug off",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k1": authACopy, "k2": authB},
			debug: false,
			want:  false,
		},
		{
			name:  "length differs, debug on",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k1": authACopy, "k2": authB},
			debug: true,
			want:  false,
		},
		{
			name:  "key missing from b, debug off",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k2": authB},
			debug: false,
			want:  false,
		},
		{
			name:  "key missing from b, debug on",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k2": authB},
			debug: true,
			want:  false,
		},
		{
			name:  "entry differs (non-exec), debug off",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k1": authB},
			debug: false,
			want:  false,
		},
		{
			name:  "entry differs (non-exec), debug on",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:     map[string]*clientcmdapi.AuthInfo{"k1": authB},
			debug: true,
			want:  false,
		},
		{
			name:  "entry differs (exec), debug on",
			a:     map[string]*clientcmdapi.AuthInfo{"k1": authExecA},
			b:     map[string]*clientcmdapi.AuthInfo{"k1": authExecB},
			debug: true,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authInfoMapsEqualDebug(tt.a, tt.b, tt.debug)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConfigContentEqual_DebugDiagnostics covers configContentEqual's own
// CurrentContext comparison (the map comparisons are delegated to, and
// already covered by, the *MapsEqualDebug tests above), including the
// ATMOS_KUBECONFIG_DEBUG_DIFF-gated diagnostic branch.
func TestConfigContentEqual_DebugDiagnostics(t *testing.T) {
	info := testClusterInfo()

	t.Run("equal configs return true", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b := BuildClusterConfig(info, "dev-eks", "dev-admin")
		assert.True(t, configContentEqual(a, b))
	})

	t.Run("nil configs", func(t *testing.T) {
		assert.True(t, configContentEqual(nil, nil))
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		assert.False(t, configContentEqual(a, nil))
		assert.False(t, configContentEqual(nil, a))
	})

	t.Run("current-context differs, debug off", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b.CurrentContext = "other-context"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("current-context differs, debug on", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b.CurrentContext = "other-context"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("clusters differ propagates false", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b.Clusters[info.ARN].Server = "https://different.eks.amazonaws.com"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("contexts differ propagates false", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b.Contexts["dev-eks"].Namespace = "custom-namespace"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("auth infos differ propagates false", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b := BuildClusterConfig(info, "dev-eks", "dev-admin")
		b.AuthInfos["atmos-eks-dev-cluster-us-east-2"].Exec.Command = "different"
		assert.False(t, configContentEqual(a, b))
	})
}

// TestMergeWouldChange_DebugDiagnostics extends
// TestMergeWouldChange_StructuralComparison to specifically drive the
// ATMOS_KUBECONFIG_DEBUG_DIFF-gated diagnostic branches for every kind of
// divergence mergeWouldChange detects: current-context, and each of
// Clusters/Contexts/AuthInfos missing-key and entry-differs cases, plus the
// nested Exec-differs diagnostic within the AuthInfos case.
func TestMergeWouldChange_DebugDiagnostics(t *testing.T) {
	info := testClusterInfo()
	base := BuildClusterConfig(info, "dev-eks", "dev-admin")

	t.Run("current-context differs", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.CurrentContext = "different-context"
		assert.True(t, mergeWouldChange(base, other))
	})

	t.Run("cluster key missing from existing", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		existing := BuildClusterConfig(info, "dev-eks", "dev-admin")
		delete(existing.Clusters, info.ARN)
		assert.True(t, mergeWouldChange(existing, base))
	})

	t.Run("cluster entry differs", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.Clusters[info.ARN].Server = "https://different.eks.amazonaws.com"
		assert.True(t, mergeWouldChange(base, other))
	})

	t.Run("context key missing from existing", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		existing := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.Contexts["extra-context"] = &clientcmdapi.Context{
			Cluster:  info.ARN,
			AuthInfo: "atmos-eks-dev-cluster-us-east-2",
		}
		assert.True(t, mergeWouldChange(existing, other))
	})

	t.Run("context entry differs", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.Contexts["dev-eks"].Namespace = "custom-namespace"
		assert.True(t, mergeWouldChange(base, other))
	})

	t.Run("auth info key missing from existing", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		existing := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.AuthInfos["extra-user"] = &clientcmdapi.AuthInfo{Token: "abc"}
		assert.True(t, mergeWouldChange(existing, other))
	})

	t.Run("auth info entry differs without exec", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		existing := clientcmdapi.NewConfig()
		existing.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Token: "t1"}
		newConfig := clientcmdapi.NewConfig()
		newConfig.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Token: "t2"}
		assert.True(t, mergeWouldChange(existing, newConfig))
	})

	t.Run("auth info entry differs with exec", func(t *testing.T) {
		t.Setenv("ATMOS_KUBECONFIG_DEBUG_DIFF", "1")
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.AuthInfos["atmos-eks-dev-cluster-us-east-2"].Exec.Command = "different-command"
		assert.True(t, mergeWouldChange(base, other))
	})
}
