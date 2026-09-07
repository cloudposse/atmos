package exec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStripAffectedForUpload_PreservesRequiredFields(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:            "vpc",
			Stack:                "plat-use2-dev",
			IncludedInDependents: false,
			Settings: schema.AtmosSectionMapType{
				"pro": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	assert.Equal(t, "vpc", result[0].Component)
	assert.Equal(t, "plat-use2-dev", result[0].Stack)
	assert.Equal(t, false, result[0].IncludedInDependents)
	assert.NotNil(t, result[0].Settings)
	assert.NotNil(t, result[0].Settings["pro"])
}

func TestStripAffectedForUpload_RemovesUnusedFields(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			ComponentPath: "components/terraform/vpc",
			Namespace:     "ex1",
			Tenant:        "plat",
			Environment:   "use2",
			Stage:         "dev",
			Stack:         "plat-use2-dev",
			StackSlug:     "plat-use2-dev-vpc",
			Affected:      "stack.vars",
			Settings: schema.AtmosSectionMapType{
				"depends_on": map[string]interface{}{
					"1": map[string]interface{}{"component": "network"},
				},
				"github": map[string]interface{}{
					"actions_enabled": true,
				},
				"pro": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	// Verify removed fields are empty/zero
	assert.Empty(t, result[0].ComponentType)
	assert.Empty(t, result[0].ComponentPath)
	assert.Empty(t, result[0].Namespace)
	assert.Empty(t, result[0].Tenant)
	assert.Empty(t, result[0].Environment)
	assert.Empty(t, result[0].Stage)
	assert.Empty(t, result[0].StackSlug)
	assert.Empty(t, result[0].Affected)

	// Verify settings only contains pro
	assert.Nil(t, result[0].Settings["depends_on"])
	assert.Nil(t, result[0].Settings["github"])
	assert.NotNil(t, result[0].Settings["pro"])
}

func TestStripAffectedForUpload_RecursiveDependents(t *testing.T) {
	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Dependents: []schema.Dependent{
				{
					Component:     "database",
					ComponentType: "terraform",
					Stack:         "plat-use2-dev",
					Dependents: []schema.Dependent{
						{
							Component:     "api",
							ComponentType: "terraform",
							Stack:         "plat-use2-dev",
							Settings: schema.AtmosSectionMapType{
								"depends_on": map[string]interface{}{
									"1": map[string]interface{}{"component": "database"},
								},
								"pro": map[string]interface{}{
									"enabled": true,
								},
							},
						},
					},
					Settings: schema.AtmosSectionMapType{
						"depends_on": map[string]interface{}{
							"1": map[string]interface{}{"component": "vpc"},
						},
						"pro": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	// Check first level dependent
	assert.Len(t, result[0].Dependents, 1)
	assert.Equal(t, "database", result[0].Dependents[0].Component)
	assert.Empty(t, result[0].Dependents[0].ComponentType)
	assert.Nil(t, result[0].Dependents[0].Settings["depends_on"])
	assert.NotNil(t, result[0].Dependents[0].Settings["pro"])

	// Check nested dependent
	assert.Len(t, result[0].Dependents[0].Dependents, 1)
	assert.Equal(t, "api", result[0].Dependents[0].Dependents[0].Component)
	assert.Empty(t, result[0].Dependents[0].Dependents[0].ComponentType)
	assert.Nil(t, result[0].Dependents[0].Dependents[0].Settings["depends_on"])
	assert.NotNil(t, result[0].Dependents[0].Dependents[0].Settings["pro"])
}

func TestStripAffectedForUpload_EmptyDependents(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:  "vpc",
			Stack:      "plat-use2-dev",
			Dependents: []schema.Dependent{},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	assert.NotNil(t, result[0].Dependents)
	assert.Len(t, result[0].Dependents, 0)
}

func TestStripAffectedForUpload_NilSettings(t *testing.T) {
	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Settings:  nil,
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	assert.Nil(t, result[0].Settings)
}

func TestStripAffectedForUpload_SettingsWithoutPro(t *testing.T) {
	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Settings: schema.AtmosSectionMapType{
				"depends_on": map[string]interface{}{
					"1": map[string]interface{}{"component": "network"},
				},
				"github": map[string]interface{}{
					"actions_enabled": true,
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	// Settings should be nil when there's no pro section
	assert.Nil(t, result[0].Settings)
}

// TestStripAffectedForUpload_PreservesProEventSchema locks in the contract that
// stripSettings preserves the full settings.pro sub-tree opaquely. This is the
// only thing keeping settings.pro.merge_group.checks_requested.workflows alive
// from user YAML to the upload payload, and a future struct-tightening of
// Affected.Settings could silently drop it. Assert the per-event schema we
// support today: pull_request, release, drift_detection, and merge_group.
func TestStripAffectedForUpload_PreservesProEventSchema(t *testing.T) {
	planWorkflows := map[string]interface{}{
		"atmos-terraform-plan.yaml": map[string]interface{}{
			"inputs": map[string]interface{}{
				"component": "{{ .atmos_component }}",
				"stack":     "{{ .atmos_stack }}",
			},
		},
	}
	applyWorkflows := map[string]interface{}{
		"atmos-terraform-apply.yaml": map[string]interface{}{
			"inputs": map[string]interface{}{
				"component": "{{ .atmos_component }}",
				"stack":     "{{ .atmos_stack }}",
			},
		},
	}

	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Settings: schema.AtmosSectionMapType{
				"pro": map[string]interface{}{
					"enabled": true,
					"pull_request": map[string]interface{}{
						"opened":      map[string]interface{}{"workflows": planWorkflows},
						"synchronize": map[string]interface{}{"workflows": planWorkflows},
						"reopened":    map[string]interface{}{"workflows": planWorkflows},
						"merged":      map[string]interface{}{"workflows": applyWorkflows},
					},
					"release": map[string]interface{}{
						"published": map[string]interface{}{"workflows": applyWorkflows},
					},
					"drift_detection": map[string]interface{}{
						"enabled": true,
					},
					"merge_group": map[string]interface{}{
						"checks_requested": map[string]interface{}{"workflows": planWorkflows},
					},
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	require.Len(t, result, 1)
	pro, ok := result[0].Settings["pro"].(map[string]interface{})
	require.True(t, ok, "settings.pro must survive stripping as map[string]interface{}")

	// Each per-event block must round-trip verbatim.
	assert.Equal(t, true, pro["enabled"])
	assert.NotNil(t, pro["pull_request"], "settings.pro.pull_request must round-trip")
	assert.NotNil(t, pro["release"], "settings.pro.release must round-trip")
	assert.NotNil(t, pro["drift_detection"], "settings.pro.drift_detection must round-trip")
	assert.NotNil(t, pro["merge_group"], "settings.pro.merge_group must round-trip — required for GitHub merge-queue support")

	// Drill into merge_group to make sure the nested workflows survive too.
	mg, ok := pro["merge_group"].(map[string]interface{})
	require.True(t, ok)
	cr, ok := mg["checks_requested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, planWorkflows, cr["workflows"])
}

func TestStripAffectedForUpload_EmptyInput(t *testing.T) {
	affected := []schema.Affected{}

	result := StripAffectedForUpload(affected)

	assert.NotNil(t, result)
	assert.Len(t, result, 0)
}

// stubDescribeAffectedExec builds a describeAffectedExec whose
// target-resolution/dependents/rendering seams are stubbed to return a
// single fixed affected component, matching TestExecute_MatrixFormat's
// fixture pattern — no real git/stack resolution needed to exercise
// Execute's exec-metadata capture (research.md Decision 22).
func stubDescribeAffectedExec(atmosConfig *schema.AtmosConfiguration) describeAffectedExec {
	d := describeAffectedExec{atmosConfig: atmosConfig}
	d.IsTTYSupportForStdout = func() bool { return false }
	d.executeDescribeAffectedWithTargetRefCheckout = func(
		atmosConfig *schema.AtmosConfiguration,
		ref, sha, targetBranch string,
		includeSpaceliftAdminStacks, includeSettings bool,
		stack string, processTemplates, processYamlFunctions bool,
		skip []string, excludeLocked bool,
		authManager auth.AuthManager,
		authDisabled bool,
		errOptions DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{
			{Stack: "ue1-dev", Component: "vpc", ComponentType: "terraform"},
		}, nil, nil, "", nil
	}
	d.addDependentsToAffected = func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings, processTemplates, processFunctions bool, skip []string, onlyInStack string, authManager auth.AuthManager, authDisabled bool, errOptions DescribeStacksErrorOptions) error {
		return nil
	}
	d.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
		return nil
	}
	return d
}

// TestExecuteInner_ReturnsAffected is the regression test for research.md
// Decision 22: executeInner's affected list — previously discarded after
// computing it, leaving Execute unable to attach it as structured Data — is
// now returned to the caller. This is a signature-shape assertion: the
// returned slice must match what executeInner already computes internally
// for rendering, no new computation.
func TestExecuteInner_ReturnsAffected(t *testing.T) {
	d := stubDescribeAffectedExec(&schema.AtmosConfiguration{})

	affected, err := d.executeInner(&DescribeAffectedCmdArgs{
		Format:           "matrix",
		GithubOutputFile: t.TempDir() + "/github_output",
		CLIConfig:        &schema.AtmosConfiguration{},
	})

	require.NoError(t, err)
	require.Len(t, affected, 1)
	assert.Equal(t, "vpc", affected[0].Component)
	assert.Equal(t, "ue1-dev", affected[0].Stack)
}

// TestExecute_AttachesAffectedAsStructuredData is the regression test for
// research.md Decision 22: describe affected's execution record must carry
// its already-computed affected-stacks list as structured Data
// ({"version": 1, "stacks": [...]}), unconditionally — not gated on
// --upload, since the list is already computed for every invocation
// (unlike list instances, research.md Decision 23).
func TestExecute_AttachesAffectedAsStructuredData(t *testing.T) {
	for _, upload := range []bool{false, true} {
		t.Run("upload="+boolString(upload), func(t *testing.T) {
			t.Setenv("CI", "true")

			var mu sync.Mutex
			var received []dtos.ExecUploadRequest

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/atmos/exec") {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var req dtos.ExecUploadRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				mu.Lock()
				received = append(received, req)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success":true}`))
			}))
			defer server.Close()

			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.Settings.Pro.BaseURL = server.URL
			atmosConfig.Settings.Pro.Token = "test-token"

			d := stubDescribeAffectedExec(atmosConfig)

			err := d.Execute(&DescribeAffectedCmdArgs{
				Format:           "matrix",
				GithubOutputFile: t.TempDir() + "/github_output",
				Upload:           upload,
				CLIConfig:        atmosConfig,
			})
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			require.Len(t, received, 1, "expected exactly one exec-metadata upload for the invocation")
			require.NotNil(t, received[0].Data)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(received[0].Data, &decoded))
			assert.Equal(t, float64(1), decoded["version"])
			stacks, ok := decoded["stacks"].([]any)
			require.True(t, ok, "data.stacks must be an array")
			require.Len(t, stacks, 1)
		})
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
