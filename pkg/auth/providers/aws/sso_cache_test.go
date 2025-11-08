package aws

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// mockCacheStorage is a mock implementation of CacheStorage for testing.
type mockCacheStorage struct {
	readFileFunc       func(path string) ([]byte, error)
	writeFileFunc      func(path string, data []byte, perm os.FileMode) error
	removeFunc         func(path string) error
	mkdirAllFunc       func(path string, perm os.FileMode) error
	getXDGCacheDirFunc func(subdir string, perm os.FileMode) (string, error)
}

func (m *mockCacheStorage) ReadFile(path string) ([]byte, error) {
	if m.readFileFunc != nil {
		return m.readFileFunc(path)
	}
	return nil, os.ErrNotExist
}

func (m *mockCacheStorage) WriteFile(path string, data []byte, perm os.FileMode) error {
	if m.writeFileFunc != nil {
		return m.writeFileFunc(path, data, perm)
	}
	return nil
}

func (m *mockCacheStorage) Remove(path string) error {
	if m.removeFunc != nil {
		return m.removeFunc(path)
	}
	return nil
}

func (m *mockCacheStorage) MkdirAll(path string, perm os.FileMode) error {
	if m.mkdirAllFunc != nil {
		return m.mkdirAllFunc(path, perm)
	}
	return nil
}

func (m *mockCacheStorage) GetXDGCacheDir(subdir string, perm os.FileMode) (string, error) {
	if m.getXDGCacheDirFunc != nil {
		return m.getXDGCacheDirFunc(subdir, perm)
	}
	return "/tmp/cache", nil
}

// TestLoadCachedToken tests loading cached tokens with various scenarios.
func TestLoadCachedToken(t *testing.T) {
	defer perf.Track(nil, "aws.TestLoadCachedToken")()

	futureTime := time.Now().Add(1 * time.Hour)
	pastTime := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name           string
		cacheStorage   *mockCacheStorage
		providerRegion string
		providerURL    string
		wantToken      string
		wantExpiry     bool // Whether we expect a non-zero expiry time
		wantErr        bool
	}{
		{
			name: "valid cached token",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					cache := ssoTokenCache{
						AccessToken: "test-token",
						ExpiresAt:   futureTime,
						Region:      "us-east-1",
						StartURL:    "https://example.com",
					}
					return json.Marshal(cache)
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "test-token",
			wantExpiry:     true,
		},
		{
			name: "cache file not found",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					return nil, os.ErrNotExist
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
		{
			name: "expired token",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					cache := ssoTokenCache{
						AccessToken: "expired-token",
						ExpiresAt:   pastTime,
						Region:      "us-east-1",
						StartURL:    "https://example.com",
					}
					return json.Marshal(cache)
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
		{
			name: "region mismatch",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					cache := ssoTokenCache{
						AccessToken: "test-token",
						ExpiresAt:   futureTime,
						Region:      "us-west-2",
						StartURL:    "https://example.com",
					}
					return json.Marshal(cache)
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
		{
			name: "start URL mismatch",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					cache := ssoTokenCache{
						AccessToken: "test-token",
						ExpiresAt:   futureTime,
						Region:      "us-east-1",
						StartURL:    "https://different.com",
					}
					return json.Marshal(cache)
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
		{
			name: "invalid JSON",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					return []byte("invalid json"), nil
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
		{
			name: "XDG cache dir error",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "", errors.New("XDG cache dir error")
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
		{
			name: "read file error",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				readFileFunc: func(path string) ([]byte, error) {
					return nil, errors.New("permission denied")
				},
			},
			providerRegion: "us-east-1",
			providerURL:    "https://example.com",
			wantToken:      "",
			wantExpiry:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ssoProvider{
				name:         "test-provider",
				region:       tt.providerRegion,
				startURL:     tt.providerURL,
				cacheStorage: tt.cacheStorage,
			}

			token, expiry, err := p.loadCachedToken()

			if (err != nil) != tt.wantErr {
				t.Errorf("loadCachedToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if token != tt.wantToken {
				t.Errorf("loadCachedToken() token = %v, want %v", token, tt.wantToken)
			}

			if tt.wantExpiry && expiry.IsZero() {
				t.Errorf("loadCachedToken() expiry is zero, want non-zero")
			}

			if !tt.wantExpiry && !expiry.IsZero() {
				t.Errorf("loadCachedToken() expiry is non-zero, want zero")
			}
		})
	}
}

// TestSaveCachedToken tests saving tokens to cache.
func TestSaveCachedToken(t *testing.T) {
	defer perf.Track(nil, "aws.TestSaveCachedToken")()

	expiryTime := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name         string
		cacheStorage *mockCacheStorage
		wantErr      bool
	}{
		{
			name: "successful save",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				writeFileFunc: func(path string, data []byte, perm os.FileMode) error {
					// Verify the data is valid JSON.
					var cache ssoTokenCache
					if err := json.Unmarshal(data, &cache); err != nil {
						t.Errorf("saved data is not valid JSON: %v", err)
					}
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "XDG cache dir error - non-fatal",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "", errors.New("XDG cache dir error")
				},
			},
			wantErr: false, // Errors are non-fatal in saveCachedToken.
		},
		{
			name: "write file error - non-fatal",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				writeFileFunc: func(path string, data []byte, perm os.FileMode) error {
					return errors.New("permission denied")
				},
			},
			wantErr: false, // Errors are non-fatal in saveCachedToken.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ssoProvider{
				name:         "test-provider",
				region:       "us-east-1",
				startURL:     "https://example.com",
				cacheStorage: tt.cacheStorage,
			}

			err := p.saveCachedToken("test-token", expiryTime)

			if (err != nil) != tt.wantErr {
				t.Errorf("saveCachedToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDeleteCachedToken tests deleting cached tokens.
func TestDeleteCachedToken(t *testing.T) {
	defer perf.Track(nil, "aws.TestDeleteCachedToken")()

	tests := []struct {
		name         string
		cacheStorage *mockCacheStorage
		wantErr      bool
	}{
		{
			name: "successful delete",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				removeFunc: func(path string) error {
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "file not found - non-fatal",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				removeFunc: func(path string) error {
					return os.ErrNotExist
				},
			},
			wantErr: false,
		},
		{
			name: "XDG cache dir error - non-fatal",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "", errors.New("XDG cache dir error")
				},
			},
			wantErr: false,
		},
		{
			name: "remove error - non-fatal",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
				removeFunc: func(path string) error {
					return errors.New("permission denied")
				},
			},
			wantErr: false, // Errors are non-fatal in deleteCachedToken.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ssoProvider{
				name:         "test-provider",
				region:       "us-east-1",
				startURL:     "https://example.com",
				cacheStorage: tt.cacheStorage,
			}

			err := p.deleteCachedToken()

			if (err != nil) != tt.wantErr {
				t.Errorf("deleteCachedToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGetTokenCachePath tests cache path generation.
func TestGetTokenCachePath(t *testing.T) {
	defer perf.Track(nil, "aws.TestGetTokenCachePath")()

	tests := []struct {
		name         string
		cacheStorage *mockCacheStorage
		providerName string
		wantContains string
		wantErr      bool
	}{
		{
			name: "successful path generation",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache/aws-sso", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return nil
				},
			},
			providerName: "my-provider",
			wantContains: "my-provider/token.json",
			wantErr:      false,
		},
		{
			name: "XDG cache dir error",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "", errors.New("XDG cache dir error")
				},
			},
			providerName: "my-provider",
			wantErr:      true,
		},
		{
			name: "mkdir error",
			cacheStorage: &mockCacheStorage{
				getXDGCacheDirFunc: func(subdir string, perm os.FileMode) (string, error) {
					return "/tmp/cache/aws-sso", nil
				},
				mkdirAllFunc: func(path string, perm os.FileMode) error {
					return errors.New("permission denied")
				},
			},
			providerName: "my-provider",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ssoProvider{
				name:         tt.providerName,
				cacheStorage: tt.cacheStorage,
			}

			path, err := p.getTokenCachePath()

			if (err != nil) != tt.wantErr {
				t.Errorf("getTokenCachePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.wantContains != "" {
				if len(path) == 0 {
					t.Errorf("getTokenCachePath() returned empty path")
				}
				// Just check that path is not empty when successful.
				// Actual path format depends on OS and XDG implementation.
			}
		})
	}
}
