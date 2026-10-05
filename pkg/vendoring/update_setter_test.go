package vendoring

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/filelock"
)

func TestUpdateSourcesCustomSetterCanUsePublicLockingSetters(t *testing.T) {
	for _, kind := range []string{"vendor manifest", "component manifest"} {
		t.Run(kind, func(t *testing.T) {
			file, sources := batchUpdateFixture(t, 1)
			if kind == "component manifest" {
				file = writeComponentManifestUpdateFixture(t)
				manifest, err := ReadComponentManifest(file)
				require.NoError(t, err)
				sources = []*ResolvedSource{{File: file, Source: ComponentManifestSource(manifest, "vpc", "terraform"), FromComponentManifest: true}}
			}
			ctrl := gomock.NewController(t)
			lister := NewMockRemoteLister(ctrl)
			archived := NewMockArchivedChecker(ctrl)
			lister.EXPECT().ListTags(gomock.Any(), gomock.Any()).Return([]string{"1.0.0", "1.2.3", "1.5.0"}, nil)
			archived.EXPECT().IsArchived(gomock.Any(), gomock.Any()).Return(false, nil)
			called := false
			report, err := UpdateSourcesContext(context.Background(), nil, sources, &UpdateParams{
				MaxConcurrency: 2, Lister: lister, ArchivedChecker: archived,
				VersionSetter: func(file, component, version string) error {
					called = true
					// A bounded probe fails promptly on the old self-deadlock instead of leaving
					// a test goroutine blocked forever inside the public setter's lock.
					canonical, err := filepath.EvalSymlinks(file)
					if err != nil {
						return err
					}
					ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()
					if err := filelock.New(canonical+".lock").WithExclusive(ctx, func() error { return nil }); err != nil {
						return err
					}
					if kind == "component manifest" {
						return SetComponentManifestVersion(file, version)
					}
					return SetComponentVersion(file, component, version)
				},
			})
			require.NoError(t, err)
			assert.True(t, called)
			require.Len(t, report.Results, 1)
			assert.Equal(t, StatusUpdated, report.Results[0].Status)
			if kind == "component manifest" {
				manifest, err := ReadComponentManifest(file)
				require.NoError(t, err)
				assert.Equal(t, "1.5.0", manifest.Spec.Source.Version)
			} else {
				updated, err := readVendorSources(file)
				require.NoError(t, err)
				require.Len(t, updated, 1)
				assert.Equal(t, "1.5.0", updated[0].Version)
			}
		})
	}
}
