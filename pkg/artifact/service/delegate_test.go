package service_test

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// repoHooks overrides individual repository methods of a delegating mock. A nil hook delegates to
// the base repository.
type repoHooks struct {
	resolve func(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error)
	inspect func(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Inspection, error)
	read    func(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Content, error)
	list    func(ctx context.Context, query artifact.ListQuery) (*artifact.ListPage, error)
}

// delegatingRepository returns a generated MockRepository whose methods forward to base, except
// for the ones a hook replaces.
func delegatingRepository(t *testing.T, base artifact.Repository, hooks repoHooks) *artifact.MockRepository {
	t.Helper()
	if hooks.resolve == nil {
		hooks.resolve = base.Resolve
	}
	if hooks.inspect == nil {
		hooks.inspect = base.Inspect
	}
	if hooks.read == nil {
		hooks.read = base.Read
	}
	if hooks.list == nil {
		hooks.list = base.List
	}
	mock := artifact.NewMockRepository(gomock.NewController(t))
	mock.EXPECT().Capabilities().DoAndReturn(base.Capabilities).AnyTimes()
	mock.EXPECT().Lock(gomock.Any(), gomock.Any()).DoAndReturn(base.Lock).AnyTimes()
	mock.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(hooks.resolve).AnyTimes()
	mock.EXPECT().Inspect(gomock.Any(), gomock.Any()).DoAndReturn(hooks.inspect).AnyTimes()
	mock.EXPECT().OpenDocument(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(base.OpenDocument).AnyTimes()
	mock.EXPECT().Read(gomock.Any(), gomock.Any()).DoAndReturn(hooks.read).AnyTimes()
	mock.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(base.Publish).AnyTimes()
	mock.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(hooks.list).AnyTimes()
	mock.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(base.Delete).AnyTimes()
	mock.EXPECT().Pin(gomock.Any(), gomock.Any()).DoAndReturn(base.Pin).AnyTimes()
	mock.EXPECT().Sign(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(base.Sign).AnyTimes()
	mock.EXPECT().Verify(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(base.Verify).AnyTimes()
	return mock
}

// nativeDestination composes a repository with a generated Copier so a test can add a native
// transfer to a destination.
type nativeDestination struct {
	artifact.Repository
	artifact.Copier
}

// withoutRecord returns a copy of the inspection that carries no portable Record, like a foreign graph.
func withoutRecord(inspection *artifact.Inspection) *artifact.Inspection {
	stripped := *inspection
	stripped.Record = nil
	return &stripped
}
