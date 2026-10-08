package s3

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"

	errUtils "github.com/cloudposse/atmos/errors"
)

// bucketRegionHeader is the response header S3 sets to the bucket's real region on a redirect or
// authorization-header mismatch.
const bucketRegionHeader = "x-amz-bucket-region"

// unsupportedAuthParams are the go-getter S3 query parameters that carry credentials. The Atmos S3
// getter authenticates only through the component's Atmos identity (or the ambient AWS credential
// chain) and never reads them, so accepting them would silently ignore the credentials the user
// believes are in use.
var unsupportedAuthParams = []string{"aws_access_key_id", "aws_access_key_secret", "aws_access_token", "aws_profile"}

// rejectUnsupportedAuthParams fails when the source URL carries a credential query parameter.
func rejectUnsupportedAuthParams(u *url.URL) error {
	query := u.Query()
	found := make([]string, 0, len(unsupportedAuthParams))
	for _, param := range unsupportedAuthParams {
		if query.Has(param) {
			found = append(found, param)
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return errUtils.Build(fmt.Errorf("%w: `%s` in %s", errUtils.ErrS3SourceUnsupportedParam, found[0], displayURI(u))).
		WithHintf("S3 sources authenticate with the component's Atmos identity (`--identity` or the component's `auth` section); remove `%s` from the source URI", found[0]).
		WithContext("parameter", found[0]).
		Err()
}

// displayURI renders a source URL without its query string, so credentials and unrelated options
// never reach an error message. The region is kept by the caller when it matters.
func displayURI(u *url.URL) string {
	redacted := *u
	redacted.RawQuery = ""
	redacted.User = nil
	return redacted.String()
}

// wrapS3Error adds S3-specific diagnosis to the two failures that are otherwise indistinguishable
// from a permission problem:
//   - a redirect (301/307) or malformed-authorization 400 means the bucket lives in another region;
//     the bucket's region is read from the x-amz-bucket-region response header when present;
//   - a 403 means access was denied, or, for a principal without s3:ListBucket, that the key does
//     not exist (S3 deliberately hides missing keys from such principals).
//
// All other errors are returned unchanged so callers can still match on the underlying SDK error.
func wrapS3Error(err error, loc *sourceLocation) error {
	if err == nil {
		return nil
	}
	var respErr *awshttp.ResponseError
	if !errors.As(err, &respErr) || respErr.Response == nil || respErr.Response.Response == nil {
		return err
	}
	status, bucketRegion := respErr.HTTPStatusCode(), respErr.Response.Header.Get(bucketRegionHeader)
	switch {
	case isRegionMismatch(status, bucketRegion):
		return regionMismatchError(err, loc, bucketRegion)
	case status == http.StatusForbidden:
		return forbiddenError(err, loc)
	default:
		return err
	}
}

// isRegionMismatch reports whether a status (with the bucket region S3 reported, if any) means the
// bucket lives in another region.
func isRegionMismatch(status int, bucketRegion string) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusTemporaryRedirect:
		return true
	case http.StatusBadRequest:
		return bucketRegion != ""
	default:
		return false
	}
}

// requestRegion describes the region a request used.
func requestRegion(loc *sourceLocation) string {
	if loc.region == "" {
		return "the default AWS region"
	}
	return loc.region
}

// regionMismatchError reports a bucket that is not in the region the request used.
func regionMismatchError(err error, loc *sourceLocation, bucketRegion string) error {
	where := "an unknown region"
	hint := "Add the bucket's region to the source URI, for example `?region=us-east-2` (find it with `aws s3api get-bucket-location --bucket " + loc.bucket + "`)"
	if bucketRegion != "" {
		where = bucketRegion
		hint = fmt.Sprintf("Add `?region=%s` to the source URI", bucketRegion)
	}
	return errUtils.Build(fmt.Errorf("%w: bucket `%s` is in %s but the request used %s (%s): %w",
		errUtils.ErrS3SourceRegionMismatch, loc.bucket, where, requestRegion(loc), loc.describe(), err)).
		WithHint(hint).
		WithContext("bucket", loc.bucket).
		WithContext("key", loc.key).
		WithContext("request_region", loc.region).
		WithContext("bucket_region", bucketRegion).
		Err()
}

// forbiddenError reports a 403 and explains the missing-key ambiguity.
func forbiddenError(err error, loc *sourceLocation) error {
	return errUtils.Build(fmt.Errorf("%w: bucket `%s` key `%s` in %s (%s): %w",
		errUtils.ErrS3SourceForbidden, loc.bucket, loc.key, requestRegion(loc), loc.describe(), err)).
		WithHint("A 403 on an S3 object can mean the key does not exist: S3 returns 403 instead of 404 when the principal lacks `s3:ListBucket` on the bucket. Check the key, and grant `s3:ListBucket` to see a precise not-found error").
		WithHint("Otherwise verify the identity can `s3:GetObject` on the key (and `kms:Decrypt` for SSE-KMS objects)").
		WithContext("bucket", loc.bucket).
		WithContext("key", loc.key).
		WithContext("region", loc.region).
		Err()
}

// describe renders the source for error messages: its URI without credentials, or bucket/key.
func (l *sourceLocation) describe() string {
	if l.uri != "" {
		return "source " + l.uri
	}
	return fmt.Sprintf("source s3://%s/%s", l.bucket, l.key)
}
