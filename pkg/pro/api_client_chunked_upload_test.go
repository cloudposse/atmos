package pro

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// uploadReceipt captures real HTTP bodies, including requests rejected by the gateway.
type uploadReceipt struct {
	BatchID    string            `json:"batch_id"`
	BatchIndex *int              `json:"batch_index"`
	BatchTotal *int              `json:"batch_total"`
	Stacks     []json.RawMessage `json:"stacks"`
	Instances  []json.RawMessage `json:"instances"`
	RepoName   string            `json:"repo_name"`
	bytes      int
	rejected   bool
}

type uploadScenario struct {
	name         string
	budget       int
	serverLimit  int
	sizes        []int
	wantRequests int
	wantBatches  int
	wantError    bool
	wantSingle   bool
	wantLater413 bool
}

// testUploadByteLimits exercises both production upload APIs against a local
// gateway that rejects oversized bodies before returning any application JSON.
func testUploadByteLimits(t *testing.T, instances bool) {
	t.Helper()
	for _, scenario := range uploadScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			runUploadScenario(t, instances, &scenario)
		})
	}
}

func uploadScenarios() []uploadScenario {
	skewed := make([]int, 600)
	for i := range skewed {
		skewed[i] = 200
		if i < 200 {
			skewed[i] = 24000
		}
	}
	uniform := make([]int, 60)
	for i := range uniform {
		uniform[i] = 5000
	}
	return []uploadScenario{
		{name: "default budget with hundreds of skewed components", serverLimit: 4500000, sizes: skewed, wantBatches: 1},
		{name: "small request omits batch fields", serverLimit: 4500000, sizes: []int{100, 200}, wantRequests: 1},
		{name: "unbatched rejection restarts as a batch", budget: 1024 * 1024, serverLimit: 150 * 1024, sizes: uniform, wantBatches: 3},
		{
			name: "later rejection restarts all items", budget: 256 * 1024, serverLimit: 200 * 1024,
			sizes: []int{80000, 80000, 120000, 120000}, wantRequests: 6, wantBatches: 2, wantLater413: true,
		},
		{name: "singleton over budget accepted", budget: 1000, serverLimit: 10000, sizes: []int{2000}, wantRequests: 1, wantBatches: 1},
		{name: "oversized item isolated between small items", budget: 1000, serverLimit: 10000, sizes: []int{100, 2000, 100}, wantRequests: 3, wantBatches: 1},
		{name: "singleton rejected", budget: 1000, serverLimit: 1000, sizes: []int{2000}, wantRequests: 1, wantBatches: 1, wantError: true, wantSingle: true},
		{name: "unbatched singleton rejected", budget: 10000, serverLimit: 1000, sizes: []int{2000}, wantRequests: 1, wantError: true, wantSingle: true},
		{name: "persistent rejection stops after three reductions", budget: 1024 * 1024, serverLimit: 1, sizes: uniform, wantRequests: 4, wantBatches: 3, wantError: true},
		{name: "recovery stops at budget floor", budget: 100 * 1024, serverLimit: 1, sizes: uniform, wantRequests: 2, wantBatches: 2, wantError: true},
		{name: "budget below floor never increases", budget: 32000, serverLimit: 1, sizes: uniform, wantRequests: 1, wantBatches: 1, wantError: true},
	}
}

func runUploadScenario(t *testing.T, instances bool, scenario *uploadScenario) {
	t.Helper()
	var mu sync.Mutex
	var receipts []uploadReceipt
	path := "/api/affected-stacks"
	if instances {
		path = "/api/instances"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		var receipt uploadReceipt
		if !assert.NoError(t, json.Unmarshal(body, &receipt)) {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, path, r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		receipt.bytes = len(body)
		receipt.rejected = len(body) > scenario.serverLimit
		mu.Lock()
		receipts = append(receipts, receipt)
		mu.Unlock()
		if receipt.rejected {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	client := &AtmosProAPIClient{
		BaseURL: server.URL, BaseAPIEndpoint: "api", APIToken: "test-token",
		HTTPClient: server.Client(), MaxPayloadBytes: scenario.budget,
	}
	wantItems, err := uploadSizedItems(t, client, instances, scenario.sizes)
	assert.Equal(t, scenario.budget, client.MaxPayloadBytes, "recovery must not change configuration")
	if scenario.wantError {
		require.ErrorIs(t, err, errUtils.ErrPayloadTooLarge)
		assert.NotErrorIs(t, err, errUtils.ErrFailedToUnmarshalAPIResponse)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusRequestEntityTooLarge, apiErr.StatusCode)
		if scenario.wantSingle {
			assert.ErrorContains(t, err, `stack "stack-0" component "component-0"`)
			assert.ErrorContains(t, err, fmt.Sprintf("%d serialized bytes", len(wantItems[0])))
			assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), "Reduce large settings")
		}
	} else {
		require.NoError(t, err)
	}
	mu.Lock()
	got := append([]uploadReceipt(nil), receipts...)
	mu.Unlock()
	verifyUploadReceipts(t, got, wantItems, scenario)
}

func uploadSizedItems(t *testing.T, client *AtmosProAPIClient, instances bool, sizes []int) ([]json.RawMessage, error) {
	t.Helper()
	stacks := make([]schema.Affected, len(sizes))
	instanceItems := make([]dtos.UploadInstance, len(sizes))
	want := make([]json.RawMessage, len(sizes))
	for i, size := range sizes {
		component, stack := fmt.Sprintf("component-%d", i), fmt.Sprintf("stack-%d", i)
		settings := map[string]any{"padding": strings.Repeat("x", size)}
		stacks[i] = schema.Affected{Component: component, Stack: stack, Settings: settings}
		instanceItems[i] = dtos.UploadInstance{Component: component, Stack: stack, Settings: settings}
		var item any = stacks[i]
		if instances {
			item = instanceItems[i]
		}
		data, err := json.Marshal(item)
		require.NoError(t, err)
		want[i] = data
	}
	if instances {
		return want, client.UploadInstances(&dtos.InstancesUploadRequest{RepoName: "test-repo", Instances: instanceItems})
	}
	return want, client.UploadAffectedStacks(&dtos.UploadAffectedStacksRequest{RepoName: "test-repo", Stacks: stacks})
}

func verifyUploadReceipts(t *testing.T, receipts []uploadReceipt, want []json.RawMessage, scenario *uploadScenario) {
	t.Helper()
	require.NotEmpty(t, receipts)
	if scenario.wantRequests > 0 {
		assert.Len(t, receipts, scenario.wantRequests, "413 must never retry at the HTTP layer")
	}
	batches := map[string][]uploadReceipt{}
	budget := scenario.budget
	if budget == 0 {
		budget = DefaultMaxPayloadBytes
	}
	for _, receipt := range receipts {
		assert.Equal(t, "test-repo", receipt.RepoName)
		if len(receipt.Stacks)+len(receipt.Instances) > 1 {
			assert.LessOrEqual(t, receipt.bytes, budget, "every multi-item request must fit the current budget")
		}
		if receipt.rejected && budget > minPayloadBytes {
			budget = max(budget/2, minPayloadBytes)
		}
		if receipt.BatchID == "" {
			assert.Nil(t, receipt.BatchIndex)
			assert.Nil(t, receipt.BatchTotal)
			continue
		}
		require.NotNil(t, receipt.BatchIndex)
		require.NotNil(t, receipt.BatchTotal)
		batch := batches[receipt.BatchID]
		assert.Equal(t, len(batch), *receipt.BatchIndex)
		if len(batch) > 0 {
			assert.Equal(t, *batch[0].BatchTotal, *receipt.BatchTotal)
			assert.False(t, batch[len(batch)-1].rejected, "abandoned batches must never resume")
		}
		batches[receipt.BatchID] = append(batch, receipt)
	}
	assert.Len(t, batches, scenario.wantBatches)
	if scenario.wantLater413 {
		require.GreaterOrEqual(t, len(receipts), 3)
		assert.False(t, receipts[0].rejected)
		assert.True(t, receipts[1].rejected)
		assert.NotEqual(t, receipts[1].BatchID, receipts[2].BatchID)
		assert.Less(t, receipts[2].bytes, receipts[1].bytes)
	}
	if scenario.wantError {
		assert.True(t, receipts[len(receipts)-1].rejected)
		return
	}
	last := receipts[len(receipts)-1]
	final := batches[last.BatchID]
	if last.BatchID == "" {
		final = receipts
	} else {
		assert.Len(t, final, *last.BatchTotal, "final batch must be complete")
	}
	var actual []json.RawMessage
	for _, receipt := range final {
		assert.False(t, receipt.rejected)
		actual = append(actual, receipt.Stacks...)
		actual = append(actual, receipt.Instances...)
		if scenario.budget == 0 {
			assert.LessOrEqual(t, receipt.bytes, DefaultMaxPayloadBytes)
		}
	}
	assert.Equal(t, want, actual, "final batch must reassemble all items in original order")
}
