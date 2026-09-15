package install

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// BenchmarkInstallBatchFetchBound models eight independent fetches with a fixed
// remote latency. It reports timings without adding flaky timing assertions to tests.
func BenchmarkInstallBatchFetchBound(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "7")
			return
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = fmt.Fprint(w, "content")
	}))
	defer server.Close()
	for _, workers := range []int{1, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			config := &schema.AtmosConfiguration{BasePath: b.TempDir()}
			packages := make([]VendorPackage, 8)
			for i := range packages {
				packages[i] = NewComponentVendorPackage(&ComponentPackageParams{Name: fmt.Sprintf("component-%d", i), URI: fmt.Sprintf("%s/%d.tf", server.URL, i), ComponentPath: filepath.Join(config.BasePath, fmt.Sprint(i)), PkgType: PkgTypeRemote, IsMixin: true, MixinFilename: "main.tf"})
			}
			b.ResetTimer()
			for range b.N {
				if _, err := InstallBatch(context.Background(), config, packages, InstallOptions{MaxConcurrency: workers, RefreshLock: true}, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
