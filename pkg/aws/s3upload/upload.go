// Package s3upload uploads local files without deleting remote objects.
package s3upload

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	checksumMetadata       = "atmos-sha256"
	maxObjectSize    int64 = 5 * 1024 * 1024 * 1024
)

// Client is the S3 API needed to compare and upload objects.
type Client interface {
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// Options selects local content and a bucket/key or bucket/prefix destination.
type Options struct {
	Source       string
	Destination  string
	ContentType  string
	CacheControl string
}

// Result includes every destination, including objects already up to date.
type Result struct {
	URIs      []string
	Uploaded  int
	Unchanged int
}

type object struct {
	filename string
	key      string
	size     int64
}

// Upload compares SHA-256, size, and delivery headers before streaming changed files.
// It never creates buckets, deletes objects, or rewrites template references.
func Upload(ctx context.Context, client Client, opts Options) (Result, error) {
	defer perf.Track(nil, "s3upload.Upload")()
	result := Result{}
	bucket, prefix, err := ParseDestination(opts.Destination)
	if err != nil {
		return result, err
	}
	objects, err := planObjects(opts.Source, prefix)
	if err != nil {
		return result, fmt.Errorf("%w: %w", errUtils.ErrS3Upload, err)
	}
	for _, obj := range objects {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		changed, err := uploadObject(ctx, client, bucket, obj, opts)
		if err != nil {
			return result, fmt.Errorf("%w: s3://%s/%s: %w", errUtils.ErrS3Upload, bucket, obj.key, err)
		}
		uri := url.URL{Scheme: "s3", Host: bucket, Path: "/" + obj.key}
		result.URIs = append(result.URIs, uri.String())
		if changed {
			result.Uploaded++
		} else {
			result.Unchanged++
		}
	}
	return result, nil
}

// ParseDestination validates an S3 URI without interpreting keys as filesystem paths.
func ParseDestination(destination string) (string, string, error) {
	defer perf.Track(nil, "s3upload.ParseDestination")()
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "s3" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(u.Host, ":") {
		return "", "", fmt.Errorf("%w: destination must be s3://bucket/key or s3://bucket/prefix/", errUtils.ErrS3UploadDestination)
	}
	return u.Host, strings.TrimPrefix(u.Path, "/"), nil
}

func planObjects(source, prefix string) ([]object, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	if info.Mode().IsRegular() {
		key := prefix
		if key == "" || strings.HasSuffix(key, "/") {
			key += filepath.Base(source)
		}
		if info.Size() > maxObjectSize {
			return nil, fmt.Errorf("%w: %s exceeds 5 GiB", errUtils.ErrS3UploadSource, source)
		}
		return []object{{filename: source, key: key}}, nil
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s must be a regular file or directory", errUtils.ErrS3UploadSource, source)
	}
	return planDirectory(source, prefix)
}

func planDirectory(source, prefix string) ([]object, error) {
	var objects []object
	err := filepath.WalkDir(source, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() || fileInfo.Size() > maxObjectSize {
			return fmt.Errorf("%w: %s must be a regular file no larger than 5 GiB", errUtils.ErrS3UploadSource, name)
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		if prefix != "" {
			key = strings.TrimSuffix(prefix, "/") + "/" + key
		}
		objects = append(objects, object{filename: name, key: key})
		return nil
	})
	return objects, err
}

func uploadObject(ctx context.Context, client Client, bucket string, obj object, opts Options) (bool, error) {
	file, err := os.Open(obj.filename)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errUtils.ErrS3UploadSource
	}
	opts.Source = obj.filename
	opts.Destination = (&url.URL{Scheme: "s3", Host: bucket, Path: "/" + obj.key}).String()
	return UploadReader(ctx, client, file, info.Size(), opts)
}

func prepareReader(ctx context.Context, file io.ReadSeeker, bucket string, obj object, opts Options) (*s3.PutObjectInput, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file}); err != nil {
		return nil, err
	}
	digest := hash.Sum(nil)
	contentType, err := resolveContentType(file, obj.filename, opts.ContentType)
	if err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(obj.key), Body: file,
		ContentLength: aws.Int64(obj.size), ContentType: aws.String(contentType),
		CacheControl:   optionalString(opts.CacheControl),
		Metadata:       map[string]string{checksumMetadata: hex.EncodeToString(digest)},
		ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(digest)),
	}, nil
}

func objectMatches(remote *s3.HeadObjectOutput, input *s3.PutObjectInput) bool {
	return remote != nil && remote.Metadata[checksumMetadata] == input.Metadata[checksumMetadata] &&
		aws.ToInt64(remote.ContentLength) == aws.ToInt64(input.ContentLength) &&
		aws.ToString(remote.ContentType) == aws.ToString(input.ContentType) &&
		aws.ToString(remote.CacheControl) == aws.ToString(input.CacheControl)
}

func resolveContentType(file io.ReadSeeker, filename, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if contentType := mime.TypeByExtension(filepath.Ext(filename)); contentType != "" {
		return contentType, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	var sample [512]byte
	n, err := file.Read(sample[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return http.DetectContentType(sample[:n]), nil
}

func isMissing(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "404")
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
