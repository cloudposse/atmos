package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestStoreValuesDoNotReadDuringConstruction(t *testing.T) {
	for _, format := range []string{"yaml", "template", "unused"} {
		t.Run(format, func(t *testing.T) {
			backend := store.NewMockStore(gomock.NewController(t))
			ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": backend}}
			var value deferred.Resolver[any]
			if format == "yaml" {
				value = NewRead(ac, "!store.get remote key", "dev", nil)
				backend.EXPECT().GetKey("key").Return("resolved", nil)
			} else {
				value = NewValue(ac, nil, StoreOptions{Name: "remote", Stack: "dev", Component: "app", Key: "key"})
				if format == "unused" {
					return
				}
				backend.EXPECT().Get("dev", "app", "key").Return("resolved", nil)
			}
			got, err := value.Resolve()
			require.NoError(t, err)
			require.Equal(t, "resolved", got)
		})
	}
}
