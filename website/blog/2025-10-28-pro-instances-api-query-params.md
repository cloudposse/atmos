---
slug: pro-instances-api-query-params
title: 'Atmos Pro: Instances API Migration to Query Parameters'
authors:
  - atmos
tags:
  - bugfix
  - enhancement
date: 2025-10-28T00:00:00.000Z
release: v1.196.0
---

Updated the Atmos Pro integration to use query parameters for the instances API endpoint, fixing issues with stack and component names containing slashes and improving API compatibility.

<!--truncate-->

## What Changed

The Atmos CLI now uses query parameters instead of path parameters when communicating with the Atmos Pro instances API.

**Before:**
```
/api/v1/repos/{owner}/{repo}/instances/{stack}/{component}
```

**After:**
```
/api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}
```

## Why This Change?

This update aligns with changes made to the Atmos Pro API that provide several important improvements:

### Fixed 500 Errors for Special Characters

Previously, component names containing slashes would cause 500 errors. For example:
- Component: `eks/cluster`

These names would create ambiguous URL paths that the API couldn't parse correctly. With query parameters, these names are properly URL-encoded and handled without errors.

### Backward Compatibility

The old path-based endpoint remains available on the Atmos Pro side for backward compatibility, with deprecation headers. This ensures smooth migration for all users.

### Query Parameter Benefits

Query parameters provide:
- **Proper encoding**: Special characters like slashes are correctly URL-encoded
- **Clear structure**: No ambiguity about where the stack name ends and component name begins
- **API consistency**: Matches common REST API patterns for filtering and querying

## Impact on Users

This change is **transparent to end users**. If you're using Atmos Pro features:

- ✅ No configuration changes required
- ✅ No workflow changes needed
- ✅ Works with both old and new Atmos Pro API versions
- ✅ Automatically handles special characters in stack/component names

## Technical Details

The change updates the `UploadInstanceStatus` function in the Atmos Pro API client to use proper URL query parameter encoding:

```go
// Use query parameters for stack and component
targetURL := fmt.Sprintf("%s/%s/repos/%s/%s/instances?stack=%s&component=%s",
    c.BaseURL, c.BaseAPIEndpoint,
    url.PathEscape(dto.RepoOwner),
    url.PathEscape(dto.RepoName),
    url.QueryEscape(dto.Stack),      // Query param encoding
    url.QueryEscape(dto.Component))  // Query param encoding
```

The key change is using `url.QueryEscape()` instead of `url.PathEscape()` for stack and component values, ensuring proper encoding for use as query parameters.

## References

- [Atmos Pro Documentation](https://atmos-pro.com/docs)

---

This change improves reliability for users with complex naming conventions and ensures the Atmos CLI stays in sync with the latest Atmos Pro API improvements.
