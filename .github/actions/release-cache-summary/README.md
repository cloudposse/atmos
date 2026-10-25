# Release cache measurements

Writes the restored release cache key (or a cold-cache miss), GoReleaser elapsed
time, and uncompressed build/module cache sizes to the job summary. The caller
passes `cache-key` from `setup-go-cache` and `started-at` from a Unix-seconds
timestamp taken immediately before the GoReleaser action. Elapsed time includes
GoReleaser setup and all enabled stages; it is not a compiler-only measurement.

Call with `always()` after the build when a start timestamp exists, so failures
also report measurements. This action reads `GOCACHE` and `GOMODCACHE` established
by `setup-go-cache`; it never restores, saves, or deletes caches.
