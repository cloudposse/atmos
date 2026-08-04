package step

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	pstore "github.com/cloudposse/atmos/pkg/store"
)

func TestStoreHandlerIsRegistered(t *testing.T) {
	handler, ok := Get(storeStepType)
	require.True(t, ok)
	assert.Equal(t, storeStepType, handler.GetName())
	assert.Equal(t, CategoryCommand, handler.GetCategory())
}

func TestStoreHandlerValidate(t *testing.T) {
	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}

	tests := []struct {
		name    string
		step    *schema.WorkflowStep
		wantErr error
	}{
		{
			name:    "missing store",
			step:    &schema.WorkflowStep{Name: "s", With: map[string]any{"key": "k", "value": "v"}},
			wantErr: errUtils.ErrStepFieldRequired,
		},
		{
			name:    "missing key",
			step:    &schema.WorkflowStep{Name: "s", With: map[string]any{"store": "app", "value": "v"}},
			wantErr: errUtils.ErrStepFieldRequired,
		},
		{
			name:    "missing value",
			step:    &schema.WorkflowStep{Name: "s", With: map[string]any{"store": "app", "key": "k"}},
			wantErr: errUtils.ErrStepFieldRequired,
		},
		{
			name:    "invalid action",
			step:    &schema.WorkflowStep{Name: "s", Action: "read", With: map[string]any{"store": "app", "key": "k", "value": "v"}},
			wantErr: errUtils.ErrStoreStepInvalidAction,
		},
		{
			name: "valid",
			step: &schema.WorkflowStep{Name: "s", Action: "write", With: map[string]any{"store": "app", "key": "k", "value": "v"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(tt.step)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestStoreHandlerExecuteWritesValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := pstore.NewMockStore(ctrl)
	mockStore.EXPECT().Set("dev", "vpc", "image_tag", "sha256:abc123").Return(nil)

	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{
		Stores: pstore.StoreRegistry{"app-metadata": mockStore},
	})
	vars.Set("push", NewStepResult("app:sha256:abc123").WithMetadata("digest", "sha256:abc123"))

	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
	result, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "record-tag",
		Action: "write",
		Stack:  "dev",
		With: map[string]any{
			"store":     "app-metadata",
			"key":       "image_tag",
			"value":     "{{ .steps.push.metadata.digest }}",
			"component": "vpc",
		},
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "sha256:abc123", result.Value)
	assert.Equal(t, "app-metadata", result.Metadata["store"])
	assert.Equal(t, "image_tag", result.Metadata["key"])
	assert.Equal(t, "dev", result.Metadata["stack"])
	assert.Equal(t, "vpc", result.Metadata["component"])
}

func TestStoreHandlerExecuteAllowsSecretStore(t *testing.T) {
	// Writing generated values (e.g. a generated password) to a `secret: true` store is a
	// legitimate use case; unlike the read-side !store/!store.get functions, the write step does
	// not refuse secret-marked stores.
	ctrl := gomock.NewController(t)
	mockStore := pstore.NewMockSecretAwareStore(ctrl)
	mockStore.EXPECT().Set("", "", "generated_password", "s3cr3t").Return(nil)

	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{
		Stores: pstore.StoreRegistry{"vault": mockStore},
	})

	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
	result, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "store-password",
		Action: "write",
		With: map[string]any{
			"store": "vault",
			"key":   "generated_password",
			"value": "s3cr3t",
		},
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "s3cr3t", result.Value)
}

func TestStoreHandlerExecuteStoreNotConfigured(t *testing.T) {
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{Stores: pstore.StoreRegistry{}})

	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
	_, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "record-tag",
		Action: "write",
		With: map[string]any{
			"store": "missing",
			"key":   "k",
			"value": "v",
		},
	}, vars)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pstore.ErrStoreNotConfigured))
}

func TestStoreHandlerExecuteWriteFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := pstore.NewMockStore(ctrl)
	writeErr := errors.New("boom")
	mockStore.EXPECT().Set("", "", "k", "v").Return(writeErr)

	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{
		Stores: pstore.StoreRegistry{"app": mockStore},
	})

	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
	_, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "record-tag",
		Action: "write",
		With: map[string]any{
			"store": "app",
			"key":   "k",
			"value": "v",
		},
	}, vars)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrStoreStepWriteFailed))
	assert.True(t, errors.Is(err, writeErr))
}

func TestStoreHandlerExecuteStackComponentFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := pstore.NewMockStore(ctrl)
	mockStore.EXPECT().Set("prod", "ecs-service", "k", "v").Return(nil)

	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{
		Stores: pstore.StoreRegistry{"app": mockStore},
	})
	vars.SetFlag("stack", "prod")
	vars.SetEnv("ATMOS_COMPONENT", "ecs-service")

	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
	result, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "record-tag",
		Action: "write",
		With: map[string]any{
			"store": "app",
			"key":   "k",
			"value": "v",
		},
	}, vars)

	require.NoError(t, err)
	assert.Equal(t, "prod", result.Metadata["stack"])
	assert.Equal(t, "ecs-service", result.Metadata["component"])
}

// invalidStoreTemplate is the same invalid Go-template syntax used by
// TestVariablesResolveExecutionError in variables_test.go to force a
// vars.Resolve failure -- an unterminated {{ range }} action never gets
// {{ end }}, so template.Parse fails.
const invalidStoreTemplate = "{{ range .steps }}{{ . }}"

// TestStoreHandlerExecuteResolveErrors exercises every vars.Resolve failure
// branch in StoreHandler.Execute (store.go:85-104): store, key, value, stack,
// and component each get their own wrapped "failed to resolve <field>" error.
func TestStoreHandlerExecuteResolveErrors(t *testing.T) {
	tests := []struct {
		name    string
		step    *schema.WorkflowStep
		wantMsg string
	}{
		{
			name: "invalid store template",
			step: &schema.WorkflowStep{
				Name:   "s",
				Action: "write",
				With: map[string]any{
					"store": invalidStoreTemplate,
					"key":   "k",
					"value": "v",
				},
			},
			wantMsg: "failed to resolve store",
		},
		{
			name: "invalid key template",
			step: &schema.WorkflowStep{
				Name:   "s",
				Action: "write",
				With: map[string]any{
					"store": "app",
					"key":   invalidStoreTemplate,
					"value": "v",
				},
			},
			wantMsg: "failed to resolve key",
		},
		{
			name: "invalid value template",
			step: &schema.WorkflowStep{
				Name:   "s",
				Action: "write",
				With: map[string]any{
					"store": "app",
					"key":   "k",
					"value": invalidStoreTemplate,
				},
			},
			wantMsg: "failed to resolve value",
		},
		{
			name: "invalid stack template",
			step: &schema.WorkflowStep{
				Name:   "s",
				Action: "write",
				Stack:  invalidStoreTemplate,
				With: map[string]any{
					"store": "app",
					"key":   "k",
					"value": "v",
				},
			},
			wantMsg: "failed to resolve stack",
		},
		{
			name: "invalid component template",
			step: &schema.WorkflowStep{
				Name:      "s",
				Action:    "write",
				Component: invalidStoreTemplate,
				With: map[string]any{
					"store": "app",
					"key":   "k",
					"value": "v",
				},
			},
			wantMsg: "failed to resolve component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := NewVariables()
			vars.SetAtmosConfig(&schema.AtmosConfiguration{
				Stores: pstore.StoreRegistry{},
			})

			h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
			_, err := h.Execute(context.Background(), tt.step, vars)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestStoreConfigNilStep covers the nil-step guard at store.go:129: storeConfig
// must return a zero-value storeStepConfig rather than panic on a nil step.
func TestStoreConfigNilStep(t *testing.T) {
	cfg := storeConfig(nil)
	assert.Equal(t, storeStepConfig{}, cfg)
}

// TestStoreHandlerExecuteValidateError covers the Validate failure branch at
// the top of Execute (store.go:79-81): Execute must return Validate's error
// directly without attempting to resolve or write anything.
func TestStoreHandlerExecuteValidateError(t *testing.T) {
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{Stores: pstore.StoreRegistry{}})

	h := &StoreHandler{BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false)}
	_, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "s",
		Action: "write",
		With:   map[string]any{"key": "k", "value": "v"},
	}, vars)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrStepFieldRequired))
}

// failingYAMLMarshaler implements yaml.Marshaler (gopkg.in/yaml.v3) and always
// fails, giving decodeStoreWith's yaml.Marshal call a value that returns a
// genuine error rather than panicking. A raw unsupported type like a channel
// is not usable here: yaml.v3's encoder panics with a bare string
// ("cannot marshal type: ...") for unsupported kinds instead of returning an
// error, and that panic is not of the internal yamlError type its own
// recover() unwraps, so it propagates out of yaml.Marshal uncaught and
// crashes the test. Implementing Marshaler and returning an error triggers
// yaml.v3's fail(err), which IS a yamlError, so yaml.Marshal returns the
// error gracefully -- exactly the path decodeStoreWith's error branch
// (store.go:146-149) handles.
type failingYAMLMarshaler struct{}

func (failingYAMLMarshaler) MarshalYAML() (interface{}, error) {
	return nil, errors.New("boom")
}

// TestDecodeStoreWithMarshalError covers the yaml.Marshal failure branch in
// decodeStoreWith (store.go:146-149): the function must log and return a
// zero-value storeStepConfig instead of propagating the error.
func TestDecodeStoreWithMarshalError(t *testing.T) {
	cfg := decodeStoreWith(map[string]any{"store": failingYAMLMarshaler{}})
	assert.Equal(t, storeStepConfig{}, cfg)
}

// TestDecodeStoreWithUnmarshalError covers the yaml.Unmarshal failure branch
// in decodeStoreWith (store.go:152-155): a mapping value where storeStepConfig
// expects a plain string (Store) fails yaml.Unmarshal's type check, so the
// function must log and return a zero-value storeStepConfig instead of
// propagating the error.
func TestDecodeStoreWithUnmarshalError(t *testing.T) {
	cfg := decodeStoreWith(map[string]any{"store": map[string]any{"nested": "x"}})
	assert.Equal(t, storeStepConfig{}, cfg)
}

// TestDecodeStoreWithEmpty covers the len(with) == 0 short-circuit at
// store.go:143-145 for both a nil and an empty (non-nil) with: map.
func TestDecodeStoreWithEmpty(t *testing.T) {
	assert.Equal(t, storeStepConfig{}, decodeStoreWith(nil))
	assert.Equal(t, storeStepConfig{}, decodeStoreWith(map[string]any{}))
}
