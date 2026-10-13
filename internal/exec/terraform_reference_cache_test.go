package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/degradation"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/refcache"
)

// Compile guards for the configuration fields that scope these regressions.
var (
	_ = schema.AtmosConfiguration{BasePath: "", CliConfigPath: ""}
	_ = schema.ConfigAndStacksInfo{SecretsMaskOnly: false, AuthDisabled: false}
)

// Repeated inspections and executions each retain their own successful backend read.
func TestTerraformStateCachePartitionsInspection(t *testing.T) {
	for _, deferredMode := range []bool{false, true} {
		for _, firstMask := range []bool{false, true} {
			t.Run(fmt.Sprintf("deferred=%t/inspect-first=%t", deferredMode, firstMask), func(t *testing.T) {
				ac, _, path := setupDeferredCacheTarget(t)
				if deferredMode {
					authdeferred.ConfigureAuth(ac, "false")
				} else {
					ac.AuthManager = nil
				}
				ResetStateCache()
				t.Cleanup(ResetStateCache)
				for _, maskOnly := range []bool{firstMask, !firstMask} {
					expected := fmt.Sprintf("inspect=%t", maskOnly)
					require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`{"version":4,"outputs":{"id":{"value":%q,"type":"string"}}}`, expected)), 0o600))
					value, err := GetTerraformState(ac, "!terraform.state", "dev", "target", "id", false, nil, nil, TerraformLookupOptions{SecretsMaskOnly: maskOnly})
					require.NoError(t, err)
					require.Equal(t, expected, value)
					require.NoError(t, os.WriteFile(path, []byte(`{"version":4,"outputs":{"id":{"value":"changed","type":"string"}}}`), 0o600))
					for range 3 {
						value, err = GetTerraformState(ac, "!terraform.state", "dev", "target", "id", false, nil, nil, TerraformLookupOptions{SecretsMaskOnly: maskOnly})
						require.NoError(t, err)
						require.Equal(t, expected, value, "a repeated lookup must reuse the backend read in this mode")
					}
				}
			})
		}
	}
}

func TestComponentCachePartitionsInspection(t *testing.T) {
	for _, deferredMode := range []bool{false, true} {
		for _, firstMask := range []bool{false, true} {
			t.Run(fmt.Sprintf("deferred=%t/inspect-first=%t", deferredMode, firstMask), func(t *testing.T) {
				ac, _, _ := setupDeferredCacheTarget(t)
				if deferredMode {
					authdeferred.ConfigureAuth(ac, "false")
				} else {
					ac.AuthManager = nil
				}
				componentFuncSyncMap.Clear()
				t.Cleanup(componentFuncSyncMap.Clear)
				executor := NewMockComponentFuncOutputsExecutor(gomock.NewController(t))
				original := componentFuncOutputsExecutor
				componentFuncOutputsExecutor = executor
				t.Cleanup(func() { componentFuncOutputsExecutor = original })
				for _, maskOnly := range []bool{firstMask, !firstMask} {
					expected := fmt.Sprintf("inspect=%t", maskOnly)
					executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": expected}, nil).Times(1)
					for range 4 {
						value, err := componentFunc(ac, &schema.ConfigAndStacksInfo{SecretsMaskOnly: maskOnly, AuthDisabled: true}, "target", "dev")
						require.NoError(t, err)
						require.Equal(t, expected, value.(map[string]any)["outputs"].(map[string]any)["id"])
					}
				}
			})
		}
	}
}

// Secret inspection remains provider-free even when either mode has a warm cache.
func TestReferenceCacheInspectionDoesNotReadSecrets(t *testing.T) {
	for _, deferredMode := range []bool{false, true} {
		for _, firstMask := range []bool{false, true} {
			for _, name := range []string{"state", "component"} {
				t.Run(fmt.Sprintf("%s/deferred=%t/inspect-first=%t", name, deferredMode, firstMask), func(t *testing.T) {
					ac, store := setupNativeSecretReference(t)
					if deferredMode {
						authdeferred.ConfigureAuth(ac, "false")
					}
					inspecting := false
					calls := 0
					store.EXPECT().Get("dev", "producer", "CREDENTIAL").DoAndReturn(func(_, _, _ string) (any, error) {
						require.False(t, inspecting, "inspection must not read the secret provider")
						calls++
						return nativeReferenceSecret, nil
					}).MinTimes(1)
					for _, maskOnly := range []bool{firstMask, !firstMask, firstMask} {
						inspecting = maskOnly
						expected := nativeReferenceSecret
						if maskOnly {
							expected = iolib.MaskReplacement
						}
						for range 3 {
							value, err := loadReferenceForCacheTest(ac, name, maskOnly)
							require.NoError(t, err)
							require.Equal(t, expected, value)
						}
					}
					require.Positive(t, calls)
				})
			}
		}
	}
}

func loadReferenceForCacheTest(ac *schema.AtmosConfiguration, name string, maskOnly bool) (any, error) {
	if name == "state" {
		return GetTerraformState(ac, "!terraform.state", "dev", "producer", "credential", false, nil, nil, TerraformLookupOptions{SecretsMaskOnly: maskOnly})
	}
	sections, err := componentFunc(ac, &schema.ConfigAndStacksInfo{SecretsMaskOnly: maskOnly}, "producer", "dev")
	if err != nil {
		return nil, err
	}
	return sections.(map[string]any)["outputs"].(map[string]any)["credential"], nil
}

// HEAD and BASE can have the same target name and different static configuration.
func TestReferenceCacheSeparatesStaticConfiguration(t *testing.T) {
	for _, name := range []string{"state", "component"} {
		for _, maskOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/inspect=%t", name, maskOnly), func(t *testing.T) {
				first, _ := setupNativeSecretReference(t)
				second, _ := setupNativeSecretReference(t)
				for _, target := range []struct {
					config *schema.AtmosConfiguration
					value  string
				}{{first, "HEAD"}, {second, "BASE"}} {
					path := filepath.Join(target.config.BasePath, "stacks", "dev.yaml")
					content, err := os.ReadFile(path)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(content), "!secret CREDENTIAL", target.value)), 0o600))
				}
				for _, target := range []struct {
					config *schema.AtmosConfiguration
					value  string
				}{{first, "HEAD"}, {second, "BASE"}, {first, "HEAD"}, {second, "BASE"}} {
					value, err := loadReferenceForCacheTest(target.config, name, maskOnly)
					require.NoError(t, err)
					require.Equal(t, target.value, value)
				}
			})
		}
	}
}

func TestInvalidateTerraformStateCacheAllPartitions(t *testing.T) {
	ResetStateCache()
	t.Cleanup(ResetStateCache)
	keys := []refcache.Key{}
	for _, basePath := range []string{"head", "base"} {
		for _, configPath := range []string{"config-first", "config-second"} {
			for _, maskOnly := range []bool{false, true} {
				for _, authDisabled := range []bool{false, true} {
					key := refcache.NewKey(&schema.AtmosConfiguration{BasePath: basePath, CliConfigPath: configPath}, "dev", "target")
					key.Inspection = maskOnly
					key.Auth = refcache.AuthScope(nil, nil, authDisabled)
					keys = append(keys, key)
					terraformStateCache.Store(key, terraformStateNotProvisionedCacheEntry{})
				}
			}
		}
	}
	unrelated := refcache.NewKey(&schema.AtmosConfiguration{}, "other", "target")
	terraformStateCache.Store(unrelated, terraformStateNotProvisionedCacheEntry{})
	invalidateTerraformStateCache("dev", "target")
	for _, key := range keys {
		_, exists := terraformStateCache.Load(key)
		require.False(t, exists)
	}
	_, exists := terraformStateCache.Load(unrelated)
	require.True(t, exists, "another stack must retain its cached state")
}

func TestComponentInspectionRetriesComputedValues(t *testing.T) {
	for _, deferredMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("deferred=%t", deferredMode), func(t *testing.T) {
			ac, _, _ := setupDeferredCacheTarget(t)
			if deferredMode {
				authdeferred.ConfigureAuth(ac, "false")
			} else {
				ac.AuthManager = nil
			}
			componentFuncSyncMap.Clear()
			t.Cleanup(componentFuncSyncMap.Clear)
			executor := NewMockComponentFuncOutputsExecutor(gomock.NewController(t))
			original := componentFuncOutputsExecutor
			componentFuncOutputsExecutor = executor
			t.Cleanup(func() { componentFuncOutputsExecutor = original })
			gomock.InOrder(
				executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": degradation.AtmosComputedValue{}}, nil),
				executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": "ready"}, nil),
			)
			for _, expected := range []any{degradation.AtmosComputedValue{}, "ready", "ready"} {
				value, err := componentFunc(ac, &schema.ConfigAndStacksInfo{SecretsMaskOnly: true, AuthDisabled: true}, "target", "dev")
				require.NoError(t, err)
				require.Equal(t, expected, value.(map[string]any)["outputs"].(map[string]any)["id"])
			}
		})
	}
}

func TestReferenceCacheSeparatesConfigPath(t *testing.T) {
	for _, name := range []string{"state", "component"} {
		t.Run(name, func(t *testing.T) {
			ac, _, path := setupDeferredCacheTarget(t)
			ac.AuthManager = nil
			ResetStateCache()
			componentFuncSyncMap.Clear()
			t.Cleanup(ResetStateCache)
			t.Cleanup(componentFuncSyncMap.Clear)
			executor := NewMockComponentFuncOutputsExecutor(gomock.NewController(t))
			original := componentFuncOutputsExecutor
			componentFuncOutputsExecutor = executor
			t.Cleanup(func() { componentFuncOutputsExecutor = original })
			for _, configPath := range []string{"first", "second"} {
				ac.CliConfigPath = configPath
				require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`{"version":4,"outputs":{"id":{"value":%q,"type":"string"}}}`, configPath)), 0o600))
				if name == "component" {
					executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": configPath}, nil).Times(1)
				}
				for range 2 {
					if name == "state" {
						value, err := GetTerraformState(ac, "!terraform.state", "dev", "target", "id", false, nil, nil, TerraformLookupOptions{SecretsMaskOnly: true})
						require.NoError(t, err)
						require.Equal(t, configPath, value)
					} else {
						value, err := componentFunc(ac, &schema.ConfigAndStacksInfo{SecretsMaskOnly: true, AuthDisabled: true}, "target", "dev")
						require.NoError(t, err)
						require.Equal(t, configPath, value.(map[string]any)["outputs"].(map[string]any)["id"])
					}
				}
			}
		})
	}
}

func TestStateInspectionCachePreservesMissingStaticOutput(t *testing.T) {
	ac, _ := setupNativeSecretReference(t)
	for range 2 {
		_, err := GetTerraformState(ac, "!terraform.state", "dev", "producer", "missing", false, nil, nil, TerraformLookupOptions{SecretsMaskOnly: true})
		require.ErrorIs(t, err, errUtils.ErrReadTerraformState, "cold and warm static lookups must both reject missing outputs")
	}
}
