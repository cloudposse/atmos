package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// These tests exercise the no-op detection helpers' diagnostic-logging
// branches. The diagnostics only emit a structured log line (unconditionally,
// visible with ATMOS_LOGS_LEVEL=Debug), so there's nothing to assert about
// the log output itself — the point is to drive execution through every
// branch (map length differs, key missing, entry differs, and the nested
// Exec-differs case) in addition to locking in the boolean contract of each
// comparison helper.

func TestClusterMapsEqual(t *testing.T) {
	clusterA := &clientcmdapi.Cluster{Server: "https://a"}
	clusterACopy := &clientcmdapi.Cluster{Server: "https://a"}
	clusterB := &clientcmdapi.Cluster{Server: "https://b"}

	tests := []struct {
		name string
		a, b map[string]*clientcmdapi.Cluster
		want bool
	}{
		{
			name: "equal maps",
			a:    map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:    map[string]*clientcmdapi.Cluster{"k1": clusterACopy},
			want: true,
		},
		{
			name: "length differs",
			a:    map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:    map[string]*clientcmdapi.Cluster{"k1": clusterACopy, "k2": clusterB},
			want: false,
		},
		{
			name: "key missing from b",
			a:    map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:    map[string]*clientcmdapi.Cluster{"k2": clusterB},
			want: false,
		},
		{
			name: "entry differs",
			a:    map[string]*clientcmdapi.Cluster{"k1": clusterA},
			b:    map[string]*clientcmdapi.Cluster{"k1": clusterB},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterMapsEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContextMapsEqual(t *testing.T) {
	ctxA := &clientcmdapi.Context{Cluster: "c1", AuthInfo: "a1"}
	ctxACopy := &clientcmdapi.Context{Cluster: "c1", AuthInfo: "a1"}
	ctxB := &clientcmdapi.Context{Cluster: "c2", AuthInfo: "a1"}

	tests := []struct {
		name string
		a, b map[string]*clientcmdapi.Context
		want bool
	}{
		{
			name: "equal maps",
			a:    map[string]*clientcmdapi.Context{"k1": ctxA},
			b:    map[string]*clientcmdapi.Context{"k1": ctxACopy},
			want: true,
		},
		{
			name: "length differs",
			a:    map[string]*clientcmdapi.Context{"k1": ctxA},
			b:    map[string]*clientcmdapi.Context{"k1": ctxACopy, "k2": ctxB},
			want: false,
		},
		{
			name: "key missing from b",
			a:    map[string]*clientcmdapi.Context{"k1": ctxA},
			b:    map[string]*clientcmdapi.Context{"k2": ctxB},
			want: false,
		},
		{
			name: "entry differs",
			a:    map[string]*clientcmdapi.Context{"k1": ctxA},
			b:    map[string]*clientcmdapi.Context{"k1": ctxB},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contextMapsEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuthInfoMapsEqual(t *testing.T) {
	authA := &clientcmdapi.AuthInfo{Token: "t1"}
	authACopy := &clientcmdapi.AuthInfo{Token: "t1"}
	authB := &clientcmdapi.AuthInfo{Token: "t2"}
	authExecA := &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "atmos"}}
	authExecB := &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "other"}}

	tests := []struct {
		name string
		a, b map[string]*clientcmdapi.AuthInfo
		want bool
	}{
		{
			name: "equal maps",
			a:    map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:    map[string]*clientcmdapi.AuthInfo{"k1": authACopy},
			want: true,
		},
		{
			name: "length differs",
			a:    map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:    map[string]*clientcmdapi.AuthInfo{"k1": authACopy, "k2": authB},
			want: false,
		},
		{
			name: "key missing from b",
			a:    map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:    map[string]*clientcmdapi.AuthInfo{"k2": authB},
			want: false,
		},
		{
			name: "entry differs (non-exec)",
			a:    map[string]*clientcmdapi.AuthInfo{"k1": authA},
			b:    map[string]*clientcmdapi.AuthInfo{"k1": authB},
			want: false,
		},
		{
			name: "entry differs (exec)",
			a:    map[string]*clientcmdapi.AuthInfo{"k1": authExecA},
			b:    map[string]*clientcmdapi.AuthInfo{"k1": authExecB},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authInfoMapsEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConfigContentEqual_Diagnostics covers configContentEqual's own
// CurrentContext comparison (the map comparisons are delegated to, and
// already covered by, the *MapsEqual tests above), including the diagnostic
// logging branch.
func TestConfigContentEqual_Diagnostics(t *testing.T) {
	info := testClusterInfo()

	t.Run("equal configs return true", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks")
		b := BuildClusterConfig(info, "dev-eks")
		assert.True(t, configContentEqual(a, b))
	})

	t.Run("nil configs", func(t *testing.T) {
		assert.True(t, configContentEqual(nil, nil))
		a := BuildClusterConfig(info, "dev-eks")
		assert.False(t, configContentEqual(a, nil))
		assert.False(t, configContentEqual(nil, a))
	})

	t.Run("current-context differs", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks")
		b := BuildClusterConfig(info, "dev-eks")
		b.CurrentContext = "other-context"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("clusters differ propagates false", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks")
		b := BuildClusterConfig(info, "dev-eks")
		b.Clusters[info.ID].Server = "https://different.eks.amazonaws.com"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("contexts differ propagates false", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks")
		b := BuildClusterConfig(info, "dev-eks")
		b.Contexts["dev-eks"].Namespace = "custom-namespace"
		assert.False(t, configContentEqual(a, b))
	})

	t.Run("auth infos differ propagates false", func(t *testing.T) {
		a := BuildClusterConfig(info, "dev-eks")
		b := BuildClusterConfig(info, "dev-eks")
		b.AuthInfos["atmos-eks-dev-cluster-us-east-2"].Exec.Command = "different"
		assert.False(t, configContentEqual(a, b))
	})
}

// TestMergeWouldChange_Diagnostics extends
// TestMergeWouldChange_StructuralComparison to specifically drive the
// diagnostic-logging branches for every kind of divergence mergeWouldChange
// detects: current-context, and each of Clusters/Contexts/AuthInfos
// missing-key and entry-differs cases, plus the nested Exec-differs
// diagnostic within the AuthInfos case.
func TestMergeWouldChange_Diagnostics(t *testing.T) {
	info := testClusterInfo()
	base := BuildClusterConfig(info, "dev-eks")

	t.Run("current-context differs", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks")
		other.CurrentContext = "different-context"
		assert.True(t, mergeWouldChange(base, other))
	})

	t.Run("cluster key missing from existing", func(t *testing.T) {
		existing := BuildClusterConfig(info, "dev-eks")
		delete(existing.Clusters, info.ID)
		assert.True(t, mergeWouldChange(existing, base))
	})

	t.Run("cluster entry differs", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks")
		other.Clusters[info.ID].Server = "https://different.eks.amazonaws.com"
		assert.True(t, mergeWouldChange(base, other))
	})

	t.Run("context key missing from existing", func(t *testing.T) {
		existing := BuildClusterConfig(info, "dev-eks")
		other := BuildClusterConfig(info, "dev-eks")
		other.Contexts["extra-context"] = &clientcmdapi.Context{
			Cluster:  info.ID,
			AuthInfo: "atmos-eks-dev-cluster-us-east-2",
		}
		assert.True(t, mergeWouldChange(existing, other))
	})

	t.Run("context entry differs", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks")
		other.Contexts["dev-eks"].Namespace = "custom-namespace"
		assert.True(t, mergeWouldChange(base, other))
	})

	t.Run("auth info key missing from existing", func(t *testing.T) {
		existing := BuildClusterConfig(info, "dev-eks")
		other := BuildClusterConfig(info, "dev-eks")
		other.AuthInfos["extra-user"] = &clientcmdapi.AuthInfo{Token: "abc"}
		assert.True(t, mergeWouldChange(existing, other))
	})

	t.Run("auth info entry differs without exec", func(t *testing.T) {
		existing := clientcmdapi.NewConfig()
		existing.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Token: "t1"}
		newConfig := clientcmdapi.NewConfig()
		newConfig.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Token: "t2"}
		assert.True(t, mergeWouldChange(existing, newConfig))
	})

	t.Run("auth info entry differs with exec", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks")
		other.AuthInfos["atmos-eks-dev-cluster-us-east-2"].Exec.Command = "different-command"
		assert.True(t, mergeWouldChange(base, other))
	})
}
