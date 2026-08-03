package store

import (
	"testing"

	pstore "github.com/cloudposse/atmos/pkg/store"
)

// setCall records a single Set invocation on the fake service.
type setCall struct {
	name, stack, component, key string
	value                       any
}

// getCall records a single Get invocation on the fake service.
type getCall struct {
	name, stack, component, key string
}

// deleteCall records a single Delete invocation on the fake service.
type deleteCall struct {
	name, stack, component, key string
}

// fakeStoreService is a configurable, call-recording implementation of storeService used to drive
// the store command handlers without constructing real config/auth/backends.
type fakeStoreService struct {
	// Configurable return values.
	getValues map[string]any
	getErrs   map[string]error
	descs     []pstore.Descriptor

	// Configurable errors.
	setErr    error
	deleteErr error

	// Recorded calls.
	setCalls    []setCall
	getCalls    []getCall
	deleteCalls []deleteCall
}

// newFakeStoreService returns a fake with empty maps ready for configuration.
func newFakeStoreService() *fakeStoreService {
	return &fakeStoreService{
		getValues: map[string]any{},
		getErrs:   map[string]error{},
	}
}

func (f *fakeStoreService) Set(name, stack, component, key string, value any) error {
	f.setCalls = append(f.setCalls, setCall{name: name, stack: stack, component: component, key: key, value: value})
	return f.setErr
}

func (f *fakeStoreService) Get(name, stack, component, key string) (any, error) {
	f.getCalls = append(f.getCalls, getCall{name: name, stack: stack, component: component, key: key})
	if err, ok := f.getErrs[key]; ok {
		return nil, err
	}
	return f.getValues[key], nil
}

func (f *fakeStoreService) Delete(name, stack, component, key string) error {
	f.deleteCalls = append(f.deleteCalls, deleteCall{name: name, stack: stack, component: component, key: key})
	return f.deleteErr
}

func (f *fakeStoreService) List() []pstore.Descriptor { return f.descs }

// installService overrides loadServiceFn to return the given fake (or loadErr when set) and
// restores the original via t.Cleanup.
func installService(t *testing.T, svc storeService, loadErr error) {
	t.Helper()

	orig := loadServiceFn
	loadServiceFn = func(_ storeScope) (storeService, error) {
		if loadErr != nil {
			return nil, loadErr
		}
		return svc, nil
	}
	t.Cleanup(func() { loadServiceFn = orig })

	// `store list` loads via loadServiceForListFn; override it too so list tests use the same
	// fake.
	origList := loadServiceForListFn
	loadServiceForListFn = func(_ storeScope) (storeService, error) {
		if loadErr != nil {
			return nil, loadErr
		}
		return svc, nil
	}
	t.Cleanup(func() { loadServiceForListFn = origList })
}

// overridePromptForValue overrides promptForValueFn and restores it via t.Cleanup.
func overridePromptForValue(t *testing.T, value string, err error) {
	t.Helper()

	orig := promptForValueFn
	promptForValueFn = func() (string, error) { return value, err }
	t.Cleanup(func() { promptForValueFn = orig })
}

// overrideConfirmAction overrides confirmActionFn and restores it via t.Cleanup. It records the
// titles it was asked to confirm in the returned slice pointer.
func overrideConfirmAction(t *testing.T, confirmed bool, err error) *[]string {
	t.Helper()

	titles := &[]string{}
	orig := confirmActionFn
	confirmActionFn = func(title string) (bool, error) {
		*titles = append(*titles, title)
		return confirmed, err
	}
	t.Cleanup(func() { confirmActionFn = orig })
	return titles
}
