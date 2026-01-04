package cache

import "errors"

var (
	// ErrCacheMiss indicates the requested key was not found in cache.
	ErrCacheMiss = errors.New("cache miss")

	// ErrCacheExpired indicates the cached entry has expired.
	ErrCacheExpired = errors.New("cache expired")
)
