// Package s3 implements authenticated S3 sources for go-getter.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	directoryMode = 0o755
	fileMode      = 0o644
)

type sourceAWSAuthKey struct{}

type sourceAWSAuthResolverKey struct{}

// WithAuthResolver defers authentication until an S3 source actually needs downloading.
func WithAuthResolver(ctx context.Context, resolver func(context.Context) (*schema.AWSAuthContext, error)) context.Context {
	defer perf.Track(nil, "s3.WithAuthResolver")()
	return context.WithValue(ctx, sourceAWSAuthResolverKey{}, resolver)
}

// WithAuthContext scopes source-download credentials to one request without changing the environment.
func WithAuthContext(ctx context.Context, auth *schema.AWSAuthContext) context.Context {
	defer perf.Track(nil, "s3.WithAuthContext")()
	if auth == nil {
		return ctx
	}
	snapshot := *auth
	return context.WithValue(ctx, sourceAWSAuthKey{}, &snapshot)
}

type sourceClient interface {
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type sourceLocation struct{ bucket, key, region, endpoint, version string }

// Getter downloads S3 objects and prefixes with request-scoped authentication.
type Getter struct {
	authOnce  sync.Once
	auth      *schema.AWSAuthContext
	authErr   error
	ctx       context.Context
	newClient func(context.Context, *sourceLocation) (sourceClient, error)
}

// NewGetter creates a private getter for one go-getter client.
func NewGetter(ctx context.Context) *Getter {
	defer perf.Track(nil, "s3.NewGetter")()
	g := &Getter{ctx: ctx}
	g.newClient = g.buildClient
	return g
}

// buildClient creates an AWS SDK client using credentials resolved once for this download.
func (g *Getter) buildClient(ctx context.Context, loc *sourceLocation) (sourceClient, error) {
	g.authOnce.Do(func() { g.auth, g.authErr = resolveAuth(ctx) })
	if g.authErr != nil {
		return nil, g.authErr
	}
	config, err := identity.LoadConfigWithAuth(ctx, loc.region, "", 0, g.auth)
	if err != nil {
		return nil, err
	}
	endpoint := loc.endpoint
	if g.auth != nil && g.auth.EndpointURL != "" {
		endpoint = g.auth.EndpointURL
	}
	return s3.NewFromConfig(config, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	}), nil
}

// resolveAuth prefers explicit request credentials over a deferred resolver.
func resolveAuth(ctx context.Context) (*schema.AWSAuthContext, error) {
	if auth, ok := ctx.Value(sourceAWSAuthKey{}).(*schema.AWSAuthContext); ok && auth != nil {
		return auth, nil
	}
	if resolve, ok := ctx.Value(sourceAWSAuthResolverKey{}).(func(context.Context) (*schema.AWSAuthContext, error)); ok && resolve != nil {
		return resolve(ctx)
	}
	return nil, nil
}

// SetClient accepts the owning client context before any download begins.
func (g *Getter) SetClient(c *getter.Client) {
	defer perf.Track(nil, "s3.Getter.SetClient")()
	if c.Ctx != nil {
		g.ctx = c.Ctx
	}
}

// resolve parses a source URL and constructs its scoped client.
func (g *Getter) resolve(u *url.URL) (sourceClient, *sourceLocation, error) {
	loc, err := parseS3SourceURL(u)
	if err != nil {
		return nil, loc, err
	}
	client, err := g.newClient(g.ctx, loc)
	return client, loc, err
}

// ClientMode checks exact objects before treating a prefix as a directory.
func (g *Getter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	defer perf.Track(nil, "s3.Getter.ClientMode")()
	client, loc, err := g.resolve(u)
	if err != nil {
		return 0, err
	}
	// A version identifies one object unambiguously; never turn its lookup into
	// a prefix listing (which would apply the same version to unrelated keys).
	if loc.version != "" {
		return getter.ClientModeFile, nil
	}
	if loc.key == "" || strings.HasSuffix(loc.key, "/") {
		return getter.ClientModeDir, nil
	}
	head := &s3.HeadObjectInput{Bucket: aws.String(loc.bucket), Key: aws.String(loc.key)}
	if _, err := client.HeadObject(g.ctx, head); err == nil {
		return getter.ClientModeFile, nil
	} else if !objectNotFound(err) {
		return 0, err
	}
	return g.prefixMode(client, loc)
}

// objectNotFound distinguishes missing objects from authorization and transport failures.
func objectNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NotFound", "NoSuchKey", "404":
		return true
	default:
		return false
	}
}

// prefixMode recognizes only exact keys or descendants within the requested prefix.
func (g *Getter) prefixMode(client sourceClient, loc *sourceLocation) (getter.ClientMode, error) {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(loc.bucket), Prefix: aws.String(loc.key)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(g.ctx)
		if err != nil {
			return 0, err
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if key == loc.key {
				return getter.ClientModeFile, nil
			}
			if strings.HasPrefix(key, strings.TrimSuffix(loc.key, "/")+"/") {
				return getter.ClientModeDir, nil
			}
		}
	}
	return getter.ClientModeFile, nil
}

// GetFile downloads a single object, honoring an optional version ID.
func (g *Getter) GetFile(dst string, u *url.URL) error {
	defer perf.Track(nil, "s3.Getter.GetFile")()
	client, loc, err := g.resolve(u)
	if err != nil {
		return err
	}
	return g.getObject(client, loc, dst)
}

// Get downloads a prefix while keeping object paths inside dst.
func (g *Getter) Get(dst string, u *url.URL) error {
	defer perf.Track(nil, "s3.Getter.Get")()
	client, loc, err := g.resolve(u)
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(loc.key, "/") + "/"
	if loc.key == "" {
		prefix = ""
	}
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(loc.bucket), Prefix: aws.String(prefix)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(g.ctx)
		if err != nil {
			return err
		}
		for _, obj := range page.Contents {
			if err := g.getDirectoryObject(client, loc, dst, prefix, aws.ToString(obj.Key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// getDirectoryObject rejects escaping keys before downloading a prefix member.
func (g *Getter) getDirectoryObject(client sourceClient, loc *sourceLocation, dst, prefix, key string) error {
	if strings.HasSuffix(key, "/") {
		return nil
	}
	relative := strings.TrimPrefix(key, prefix)
	if !strings.HasPrefix(key, prefix) || !filepath.IsLocal(relative) {
		return fmt.Errorf("%w: S3 object %q escapes source prefix", errUtils.ErrPathTraversal, key)
	}
	destination, err := safeS3Destination(dst, relative)
	if err != nil {
		return err
	}
	object := *loc
	object.key = key
	return g.getObject(client, &object, destination)
}

// getObject writes the selected object version and preserves download or filesystem errors.
func (g *Getter) getObject(client sourceClient, loc *sourceLocation, dst string) error {
	if info, err := os.Lstat(dst); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: refusing S3 download through symlink %q", errUtils.ErrPathTraversal, dst)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	input := &s3.GetObjectInput{Bucket: aws.String(loc.bucket), Key: aws.String(loc.key)}
	if loc.version != "" {
		input.VersionId = aws.String(loc.version)
	}
	result, err := client.GetObject(g.ctx, input)
	if err != nil {
		return err
	}
	defer result.Body.Close()
	if err := os.MkdirAll(filepath.Dir(dst), directoryMode); err != nil {
		return err
	}
	file, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, result.Body)
	return errors.Join(copyErr, file.Close())
}

// parseS3SourceURL accepts native s3 URLs and go-getter's forced HTTPS S3 URLs.
func parseS3SourceURL(u *url.URL) (*sourceLocation, error) {
	loc := &sourceLocation{region: u.Query().Get("region"), version: u.Query().Get("version")}
	if u.Scheme == "s3" {
		loc.bucket = u.Host
		loc.key = strings.TrimPrefix(u.Path, "/")
		return validateS3Source(loc)
	}
	host := u.Hostname()
	suffix := awsHostSuffix(host)
	if suffix == "" {
		loc.endpoint = u.Scheme + "://" + u.Host
		loc.bucket, loc.key, _ = strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
		return validateS3Source(loc)
	}
	service := strings.TrimSuffix(host, suffix)
	if at := strings.LastIndex(service, ".s3"); at >= 0 {
		loc.bucket, service = service[:at], service[at+1:]
		loc.key = strings.TrimPrefix(u.Path, "/")
	} else if strings.HasPrefix(service, "s3.") || strings.HasPrefix(service, "s3-") || service == "s3" {
		loc.bucket, loc.key, _ = strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	} else {
		return loc, fmt.Errorf("%w: invalid S3 host %q", errUtils.ErrDownloadFile, host)
	}
	if loc.region == "" {
		loc.region = serviceRegion(service)
	}
	return validateS3Source(loc)
}

// awsHostSuffix identifies supported AWS hostname suffixes, including China regions.
func awsHostSuffix(host string) string {
	for _, candidate := range []string{".amazonaws.com.cn", ".amazonaws.com"} {
		if strings.HasSuffix(host, candidate) {
			return candidate
		}
	}
	return ""
}

// serviceRegion extracts the region from standard and dualstack S3 service names.
func serviceRegion(service string) string {
	region := strings.TrimPrefix(strings.TrimPrefix(service, "s3"), ".")
	region = strings.TrimPrefix(region, "-")
	region = strings.TrimPrefix(region, "dualstack.")
	if region == "" {
		return "us-east-1"
	}
	return region
}

// validateS3Source requires a bucket before any AWS operation.
func validateS3Source(loc *sourceLocation) (*sourceLocation, error) {
	if loc.bucket == "" {
		return loc, fmt.Errorf("%w: S3 source must name a bucket", errUtils.ErrDownloadFile)
	}
	return loc, nil
}

// safeS3Destination refuses symlinks inside a caller-owned existing destination.
func safeS3Destination(root, relative string) (string, error) {
	current := root
	for _, part := range append([]string{""}, strings.Split(filepath.ToSlash(relative), "/")...) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: refusing S3 source path through symlink %q", errUtils.ErrPathTraversal, current)
		}
	}
	return current, nil
}
