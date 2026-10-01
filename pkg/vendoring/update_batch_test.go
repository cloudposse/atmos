package vendoring

//go:generate go run go.uber.org/mock/mockgen -destination=update_remote_mock_test.go -package=vendoring github.com/cloudposse/atmos/pkg/vendoring/version RemoteLister
//go:generate go run go.uber.org/mock/mockgen -source=archived.go -destination=update_archived_mock_test.go -package=vendoring

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/batch"
)

func batchUpdateFixture(t *testing.T, count int) (string, []*ResolvedSource) {
	t.Helper()
	var yaml strings.Builder
	yaml.WriteString("apiVersion: atmos/v1\nkind: AtmosVendorConfig\nspec:\n  sources:\n")
	for i := range count {
		fmt.Fprintf(&yaml, "    # component %d\n    - component: c%d\n      source: git::https://gitlab.com/example/c%d.git\n      version: '1.0.0' # keep pin comment\n      targets: [components/terraform/c%d]\n", i, i, i, i)
	}
	file := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yaml.String()), 0o644))
	parsed, err := readVendorSources(file)
	require.NoError(t, err)
	sources := make([]*ResolvedSource, len(parsed))
	for i := range parsed {
		sources[i] = &ResolvedSource{File: file, Source: &parsed[i]}
	}
	return file, sources
}

func TestUpdateSourcesConcurrentChecksOrderedEdits(t *testing.T) {
	file, sources := batchUpdateFixture(t, 4)
	ctrl := gomock.NewController(t)
	lister := NewMockRemoteLister(ctrl)
	archived := NewMockArchivedChecker(ctrl)
	archived.EXPECT().IsArchived(gomock.Any(), gomock.Any()).Return(false, nil).Times(4)
	started := make(chan string, 4)
	laterStarted := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var active, maximum atomic.Int32
	lister.EXPECT().ListTags(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, uri string) ([]string, error) {
		n := active.Add(1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		defer active.Add(-1)
		started <- uri
		if strings.HasSuffix(uri, "c0.git") || strings.HasSuffix(uri, "c1.git") {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if strings.HasSuffix(uri, "c2.git") {
			close(laterStarted)
		}
		if strings.HasSuffix(uri, "c0.git") {
			// c2 can start only after c1 has completed. Hold the first source
			// until then to force discovery completion out of declaration order.
			select {
			case <-laterStarted:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []string{"1.0.0", "2.0.0"}, nil
	}).Times(4)
	var events []batch.Event
	var callbacks atomic.Int32
	var edits []string
	done := make(chan struct{})
	var report *UpdateReport
	var resultErr error
	go func() {
		defer close(done)
		report, resultErr = UpdateSourcesContext(context.Background(), nil, sources, &UpdateParams{
			MaxConcurrency: 2, Lister: lister, ArchivedChecker: archived,
			OnEvent: func(e batch.Event) {
				assert.Equal(t, int32(1), callbacks.Add(1), "observers must never overlap")
				runtime.Gosched()
				events = append(events, e)
				callbacks.Add(-1)
			},
			VersionSetter: func(file, component, version string) error {
				edits = append(edits, component)
				return setComponentVersionUnlocked(file, component, version)
			},
		})
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("independent checks did not overlap")
		}
	}
	assert.Equal(t, int32(2), active.Load())
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("batch did not finish")
	}
	require.NoError(t, resultErr)
	assert.Equal(t, int32(2), maximum.Load())
	assert.Equal(t, []string{"c0", "c1", "c2", "c3"}, edits)
	require.Len(t, report.Results, 4)
	for i, res := range report.Results {
		assert.Equal(t, fmt.Sprintf("c%d", i), res.Component)
		assert.Equal(t, StatusUpdated, res.Status)
		assert.Equal(t, "2.0.0", res.LatestVersion)
	}
	require.NotEmpty(t, events)
	assert.True(t, events[0].Reset)
	assert.Equal(t, 4, events[0].Count)
	var completed []int
	for _, e := range events {
		if e.Done {
			completed = append(completed, e.ID)
			assert.Equal(t, "updated", e.Outcome)
		}
	}
	assert.Equal(t, []int{0, 1, 2, 3}, completed)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, 4, strings.Count(string(content), "version: '2.0.0' # keep pin comment"))
	for i := range 4 {
		assert.Contains(t, string(content), fmt.Sprintf("# component %d", i))
	}
}

func TestUpdateSourcesCancellationPropagates(t *testing.T) {
	file, sources := batchUpdateFixture(t, 3)
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	lister := NewMockRemoteLister(ctrl)
	archived := NewMockArchivedChecker(ctrl)
	archived.EXPECT().IsArchived(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	var exited atomic.Int32
	lister.EXPECT().ListTags(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ string) ([]string, error) {
		started <- struct{}{}
		<-ctx.Done()
		exited.Add(1)
		return nil, ctx.Err()
	}).Times(2)
	done := make(chan struct{})
	var report *UpdateReport
	var resultErr error
	var events []batch.Event
	go func() {
		defer close(done)
		report, resultErr = UpdateSourcesContext(ctx, nil, sources, &UpdateParams{MaxConcurrency: 2, Lister: lister, ArchivedChecker: archived, OnEvent: func(e batch.Event) { events = append(events, e) }})
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("checks did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("remote checks ignored cancellation")
	}
	require.ErrorIs(t, resultErr, context.Canceled)
	assert.Equal(t, int32(2), exited.Load(), "workers must exit before returning")
	require.Len(t, report.Results, 3)
	for i, res := range report.Results {
		assert.Equal(t, fmt.Sprintf("c%d", i), res.Component)
		assert.Equal(t, StatusFailed, res.Status)
		assert.Contains(t, res.Reason, "context canceled")
	}
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	var canceled []int
	for _, e := range events {
		if e.Done {
			canceled = append(canceled, e.ID)
			assert.Equal(t, "canceled", e.Outcome)
		}
	}
	assert.Equal(t, []int{0, 1, 2}, canceled)
}

func TestUpdateSourcesFailuresContinueInSourceOrder(t *testing.T) {
	_, sources := batchUpdateFixture(t, 3)
	ctrl := gomock.NewController(t)
	lister := NewMockRemoteLister(ctrl)
	archived := NewMockArchivedChecker(ctrl)
	archived.EXPECT().IsArchived(gomock.Any(), gomock.Any()).Return(false, context.DeadlineExceeded).Times(3)
	lister.EXPECT().ListTags(gomock.Any(), "https://gitlab.com/example/c0.git").Return(nil, errUtils.ErrGitLsRemoteFailed)
	lister.EXPECT().ListTags(gomock.Any(), "https://gitlab.com/example/c1.git").Return([]string{"2.0.0"}, nil)
	lister.EXPECT().ListTags(gomock.Any(), "https://gitlab.com/example/c2.git").Return([]string{"1.0.0"}, nil)
	report, err := UpdateSourcesContext(context.Background(), nil, sources, &UpdateParams{MaxConcurrency: 3, Lister: lister, ArchivedChecker: archived, VersionSetter: func(string, string, string) error { return os.ErrPermission }})
	require.ErrorIs(t, err, errUtils.ErrGitLsRemoteFailed)
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorIs(t, err, errUtils.ErrVendorUpdateFailed)
	require.Len(t, report.Results, 3)
	assert.Equal(t, StatusFailed, report.Results[0].Status)
	assert.Equal(t, StatusFailed, report.Results[1].Status)
	assert.Equal(t, StatusUpToDate, report.Results[2].Status)
	for _, res := range report.Results {
		assert.False(t, res.Archived, "archive failure remains best effort")
	}
	assert.Less(t, strings.Index(err.Error(), "c0"), strings.Index(err.Error(), "c1"), "aggregate failures retain declaration order")
}

func TestApplyUpdateProposalRejectsStaleDeclaration(t *testing.T) {
	for _, edit := range []string{"version", "source", "removed", "missing file"} {
		t.Run(edit, func(t *testing.T) {
			file, sources := batchUpdateFixture(t, 1)
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			updated := string(before)
			switch edit {
			case "version":
				updated = strings.ReplaceAll(updated, "1.0.0", "1.5.0")
			case "source":
				updated = strings.ReplaceAll(updated, "example/c0", "someone/c0")
			case "removed":
				updated = strings.ReplaceAll(updated, "component: c0", "component: replacement")
			case "missing file":
				require.NoError(t, os.Remove(file))
			}
			if edit != "missing file" {
				require.NoError(t, os.WriteFile(file, []byte(updated), 0o644))
			}
			called := false
			err = applyUpdateProposal(context.Background(), sources[0], "2.0.0", func(string, string, string) error { called = true; return nil })
			require.Error(t, err)
			assert.False(t, called, "stale discovery must never invoke the setter")
			if edit != "missing file" {
				require.ErrorIs(t, err, errUtils.ErrVendorUpdateFailed)
				after, readErr := os.ReadFile(file)
				require.NoError(t, readErr)
				assert.Equal(t, updated, string(after))
			}
		})
	}
}

func TestApplyUpdateProposalHonorsLockCancellation(t *testing.T) {
	file, sources := batchUpdateFixture(t, 1)
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- filelock.New(file+".lock").WithExclusive(context.Background(), func() error { close(locked); <-release; return nil })
	}()
	<-locked
	defer func() { close(release); require.NoError(t, <-done) }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := applyUpdateProposal(ctx, sources[0], "2.0.0", func(string, string, string) error { called = true; return nil })
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, called)
}

func TestUpdateSourcesInvalidLimitAndEmptySelection(t *testing.T) {
	_, sources := batchUpdateFixture(t, 1)
	report, err := UpdateSourcesContext(context.Background(), nil, sources, &UpdateParams{MaxConcurrency: -1})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Nil(t, report)
	var events []batch.Event
	report, err = UpdateSourcesContext(context.Background(), nil, sources, &UpdateParams{Component: "unselected", OnEvent: func(e batch.Event) { events = append(events, e) }})
	require.NoError(t, err)
	assert.Empty(t, report.Results)
	require.Len(t, events, 1)
	assert.True(t, events[0].Reset)
	assert.Zero(t, events[0].Count)
}

func TestUpdateComponentProposalRejectsStaleDeclaration(t *testing.T) {
	for _, edit := range []string{"version", "source", "missing file"} {
		t.Run(edit, func(t *testing.T) {
			file := writeComponentManifestUpdateFixture(t)
			original, err := ReadComponentManifest(file)
			require.NoError(t, err)
			resolved := &ResolvedSource{File: file, Source: ComponentManifestSource(original, "vpc", "terraform"), FromComponentManifest: true}
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			switch edit {
			case "version":
				require.NoError(t, os.WriteFile(file, []byte(strings.ReplaceAll(string(before), "1.2.3", "1.3.0")), 0o644))
			case "source":
				require.NoError(t, os.WriteFile(file, []byte(strings.ReplaceAll(string(before), "cloudposse/", "different/")), 0o644))
			case "missing file":
				require.NoError(t, os.Remove(file))
			}
			called := false
			err = applyUpdateProposal(context.Background(), resolved, "2.0.0", func(string, string, string) error { called = true; return nil })
			require.Error(t, err)
			assert.False(t, called)
			if edit != "missing file" {
				require.ErrorIs(t, err, errUtils.ErrVendorUpdateFailed)
			}
		})
	}
}

func TestCheckArchivedContextHonorsCaller(t *testing.T) {
	ctrl := gomock.NewController(t)
	checker := NewMockArchivedChecker(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checker.EXPECT().IsArchived(gomock.Any(), "https://gitlab.com/example/repo.git").DoAndReturn(func(received context.Context, _ string) (bool, error) {
		require.ErrorIs(t, received.Err(), context.Canceled)
		deadline, ok := received.Deadline()
		require.True(t, ok)
		assert.LessOrEqual(t, time.Until(deadline), archivedCheckTimeout)
		return false, received.Err()
	})
	src := &schema.AtmosVendorSource{Component: "example", Source: "git::https://gitlab.com/example/repo.git", Version: "1.0.0"}
	assert.False(t, checkArchivedContext(ctx, src, checker))
}

func TestUpdateSourcesEditionSerialOrder(t *testing.T) {
	_, sources := batchUpdateFixture(t, 3)
	ctrl := gomock.NewController(t)
	lister := NewMockRemoteLister(ctrl)
	archived := NewMockArchivedChecker(ctrl)
	archived.EXPECT().IsArchived(gomock.Any(), gomock.Any()).Return(false, nil).Times(3)
	gomock.InOrder(
		lister.EXPECT().ListTags(gomock.Any(), "https://gitlab.com/example/c0.git").Return([]string{"2.0.0"}, nil),
		lister.EXPECT().ListTags(gomock.Any(), "https://gitlab.com/example/c1.git").Return([]string{"2.0.0"}, nil),
		lister.EXPECT().ListTags(gomock.Any(), "https://gitlab.com/example/c2.git").Return([]string{"2.0.0"}, nil),
	)
	var transitions []string
	config := &schema.AtmosConfiguration{Vendor: schema.Vendor{MaxConcurrency: 1}}
	report, err := UpdateSourcesContext(context.Background(), config, sources, &UpdateParams{
		Lister: lister, ArchivedChecker: archived, DryRun: true,
		OnEvent: func(e batch.Event) {
			if !e.Reset {
				if e.Done {
					transitions = append(transitions, "done:"+e.Label)
				} else {
					transitions = append(transitions, "check:"+e.Label)
				}
			}
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, report.UpdatedCount())
	assert.Equal(t, []string{"check:c0", "done:c0", "check:c1", "done:c1", "check:c2", "done:c2"}, transitions)
}

func TestCheckArchivedCompatibilityWrapper(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(fmt.Sprint(archived), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			checker := NewMockArchivedChecker(ctrl)
			checker.EXPECT().IsArchived(gomock.Any(), "https://gitlab.com/example/repo.git").Return(archived, nil)
			src := &schema.AtmosVendorSource{Component: "example", Source: "git::https://gitlab.com/example/repo.git", Version: "1.0.0"}
			assert.Equal(t, archived, checkArchived(src, checker))
		})
	}
}

func TestUpdateProgressDistinguishesDuplicateComponentNames(t *testing.T) {
	sources := []*ResolvedSource{
		{File: "terraform/component.yaml", ComponentType: "terraform", Source: &schema.AtmosVendorSource{Component: "vpc"}},
		{File: "helmfile/component.yaml", ComponentType: "helmfile", Source: &schema.AtmosVendorSource{Component: "vpc"}},
	}
	labels := map[int]string{}
	report, err := UpdateSourcesContext(context.Background(), nil, sources, &UpdateParams{DryRun: true, OnEvent: func(event batch.Event) {
		if event.Done {
			labels[event.ID] = event.Label
		}
	}})
	require.NoError(t, err)
	require.Len(t, labels, 2)
	assert.NotEqual(t, labels[0], labels[1])
	assert.Contains(t, labels[0], "terraform/component.yaml")
	assert.Contains(t, labels[1], "helmfile/component.yaml")
	require.Len(t, report.Results, 2)
	assert.Equal(t, "vpc", report.Results[0].Component)
	assert.Equal(t, "vpc", report.Results[1].Component)
}
