package helm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"sigs.k8s.io/yaml"

	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/repo/v1"
)

const (
	repositoryLockTimeout = 30 * time.Second
	repositoryFilePerm    = 0o600
)

var (
	errHelmRepositoryLockTimeout = errors.New("timed out waiting for Helm repository lock")
	errInvalidHelmRepositoryName = errors.New("invalid Helm repository name")
)

// setupHelmRepositories materializes declarative repositories into Helm's local
// repository config/cache so repo/name chart references behave like Helm CLI.
func setupHelmRepositories(repositories []chartRepository) error {
	if len(repositories) == 0 {
		return nil
	}

	settings := newSettings()
	repoFile := settings.RepositoryConfig
	if repoFile == "" {
		return nil
	}

	unlock, err := lockRepositoryFile(repoFile)
	if err != nil {
		return err
	}
	defer unlock()

	repoConfig, err := loadRepositoryFile(repoFile)
	if err != nil {
		return err
	}

	for i := range repositories {
		item := &repositories[i]
		entry, err := repositoryEntry(item)
		if err != nil {
			return err
		}

		chartRepo, err := repo.NewChartRepository(entry, getter.All(settings, getter.WithTimeout(getter.DefaultHTTPTimeout*time.Second)))
		if err != nil {
			return err
		}
		if settings.RepositoryCache != "" {
			chartRepo.CachePath = settings.RepositoryCache
		}
		if _, err := chartRepo.DownloadIndexFile(); err != nil {
			return fmt.Errorf("looks like %q is not a valid chart repository or cannot be reached: %w", item.URL, err)
		}

		repoConfig.Update(entry)
	}

	return repoConfig.WriteFile(repoFile, repositoryFilePerm)
}

func lockRepositoryFile(repoFile string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(repoFile), os.ModePerm); err != nil && !os.IsExist(err) {
		return nil, err
	}

	lockPath := repositoryLockPath(repoFile)
	fileLock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), repositoryLockTimeout)
	locked, err := fileLock.TryLockContext(ctx, time.Second)
	cancel()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("%w: %q", errHelmRepositoryLockTimeout, lockPath)
	}

	return func() {
		_ = fileLock.Unlock()
	}, nil
}

func repositoryLockPath(repoFile string) string {
	ext := filepath.Ext(repoFile)
	if ext != "" && len(ext) < len(repoFile) {
		return strings.TrimSuffix(repoFile, ext) + ".lock"
	}
	return repoFile + ".lock"
}

func loadRepositoryFile(path string) (*repo.File, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return repo.NewFile(), nil
	}
	if err != nil {
		return nil, err
	}

	var out repo.File
	if err := yaml.Unmarshal(content, &out); err != nil {
		return nil, err
	}
	if out.Repositories == nil {
		out.Repositories = []*repo.Entry{}
	}
	return &out, nil
}

func repositoryEntry(item *chartRepository) (*repo.Entry, error) {
	if strings.Contains(item.Name, "/") {
		return nil, fmt.Errorf("%w: repository name %q contains '/'", errInvalidHelmRepositoryName, item.Name)
	}
	return &repo.Entry{
		Name:                  item.Name,
		URL:                   item.URL,
		Username:              item.Username,
		Password:              item.Password,
		PassCredentialsAll:    item.PassCredentialsAll,
		CertFile:              item.CertFile,
		KeyFile:               item.KeyFile,
		CAFile:                item.CAFile,
		InsecureSkipTLSVerify: item.InsecureSkipTLSVerify,
	}, nil
}
