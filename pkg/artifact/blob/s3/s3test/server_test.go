package s3test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func apiCode(t *testing.T, err error) string {
	t.Helper()
	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	return apiErr.ErrorCode()
}

func putObject(t *testing.T, client *s3.Client, srv *Server, key, body string, mutate ...func(*s3.PutObjectInput)) (*s3.PutObjectOutput, error) {
	t.Helper()
	in := &s3.PutObjectInput{Bucket: aws.String(srv.Bucket()), Key: aws.String(key), Body: strings.NewReader(body)}
	for _, fn := range mutate {
		fn(in)
	}
	return client.PutObject(context.Background(), in)
}

func TestConditionalPutEnforced(t *testing.T) {
	srv := NewServer(t)
	client := srv.Client()
	first, err := putObject(t, client, srv, "k", "one", func(in *s3.PutObjectInput) { in.IfNoneMatch = aws.String("*") })
	require.NoError(t, err)

	_, err = putObject(t, client, srv, "k", "two", func(in *s3.PutObjectInput) { in.IfNoneMatch = aws.String("*") })
	assert.Equal(t, "PreconditionFailed", apiCode(t, err))

	_, err = putObject(t, client, srv, "k", "three", func(in *s3.PutObjectInput) { in.IfMatch = aws.String(`"bogus"`) })
	assert.Equal(t, "PreconditionFailed", apiCode(t, err))

	_, err = putObject(t, client, srv, "k", "four", func(in *s3.PutObjectInput) { in.IfMatch = first.ETag })
	require.NoError(t, err)

	_, err = putObject(t, client, srv, "ghost", "x", func(in *s3.PutObjectInput) { in.IfMatch = aws.String(`"x"`) })
	assert.Equal(t, "NoSuchKey", apiCode(t, err), "If-Match on a missing key is a 404")
	object, _ := srv.Object("k")
	assert.Equal(t, []byte("four"), object.Data)
}

func TestConditionalBehaviorsOfNonConformingEndpoints(t *testing.T) {
	ignore := NewServer(t, WithConditionalBehavior(ConditionalIgnore))
	for range 2 {
		_, err := putObject(t, ignore.Client(), ignore, "k", "x", func(in *s3.PutObjectInput) { in.IfNoneMatch = aws.String("*") })
		require.NoError(t, err, "an ignoring endpoint overwrites silently")
	}
	notImplemented := NewServer(t, WithConditionalBehavior(ConditionalNotImplemented))
	_, err := putObject(t, notImplemented.Client(), notImplemented, "k", "x", func(in *s3.PutObjectInput) { in.IfNoneMatch = aws.String("*") })
	assert.Equal(t, "NotImplemented", apiCode(t, err))
	_, err = putObject(t, notImplemented.Client(), notImplemented, "plain", "x")
	require.NoError(t, err, "unconditional requests still work")
}

func TestListPagination(t *testing.T) {
	srv := NewServer(t, WithPageCap(4))
	for i := range 10 {
		srv.Seed(fmt.Sprintf("p/%02d", i), nil, nil)
	}
	srv.Seed("q/x", nil, nil)
	client := srv.Client()

	var keys []string
	var token *string
	pages := 0
	for {
		out, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket: aws.String(srv.Bucket()), Prefix: aws.String("p/"), MaxKeys: aws.Int32(100), ContinuationToken: token,
		})
		require.NoError(t, err)
		pages++
		assert.LessOrEqual(t, len(out.Contents), 4, "the server's page cap wins over max-keys")
		for _, obj := range out.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		require.NotEmpty(t, aws.ToString(out.NextContinuationToken))
		token = out.NextContinuationToken
	}
	assert.Len(t, keys, 10)
	assert.Equal(t, 3, pages)

	_, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String(srv.Bucket()), ContinuationToken: aws.String("!!!")})
	assert.Equal(t, "InvalidArgument", apiCode(t, err))
}

func TestListKeysWithSpecialCharactersRoundTrip(t *testing.T) {
	srv := NewServer(t)
	keys := []string{"a b/c+d", "ünï/ç", "x&y=z/1", "plain"}
	for _, key := range keys {
		srv.Seed(key, nil, nil)
	}
	out, err := srv.Client().ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String(srv.Bucket())})
	require.NoError(t, err)
	var got []string
	for _, obj := range out.Contents {
		got = append(got, aws.ToString(obj.Key))
	}
	assert.ElementsMatch(t, keys, got)
}

func TestMultipartLifecycle(t *testing.T) {
	srv := NewServer(t, WithMinPartSize(4))
	client := srv.Client()
	ctx := context.Background()
	created, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), Tagging: aws.String("a=b"), ContentType: aws.String("text/x"),
	})
	require.NoError(t, err)
	part := func(n int32, body string) types.CompletedPart {
		out, partErr := client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId, PartNumber: aws.Int32(n), Body: strings.NewReader(body),
		})
		require.NoError(t, partErr)
		return types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(n)}
	}
	p1, p2 := part(1, "aaaa"), part(2, "bb")
	assert.Len(t, srv.OpenUploads(), 1)

	_, err = client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{p2, p1}},
	})
	assert.Equal(t, "InvalidPart", apiCode(t, err), "parts must be in ascending order")

	done, err := client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{p1, p2}},
	})
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(strings.Trim(aws.ToString(done.ETag), `"`), "-2"))
	object, _ := srv.Object("m")
	assert.Equal(t, []byte("aaaabb"), object.Data)
	assert.Equal(t, map[string]string{"a": "b"}, object.Tags)
	assert.Equal(t, "text/x", object.ContentType)
	assert.Empty(t, srv.OpenUploads())

	_, err = client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId})
	assert.Equal(t, "NoSuchUpload", apiCode(t, err))
}

func TestMultipartMinimumPartSizeAndAbort(t *testing.T) {
	srv := NewServer(t, WithMinPartSize(10))
	client := srv.Client()
	ctx := context.Background()
	created, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("m")})
	require.NoError(t, err)
	var parts []types.CompletedPart
	for n := int32(1); n <= 2; n++ {
		out, partErr := client.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId, PartNumber: aws.Int32(n), Body: strings.NewReader("tiny")})
		require.NoError(t, partErr)
		parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(n)})
	}
	_, err = client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId, MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	assert.Equal(t, "InvalidPart", apiCode(t, err), "a non-final part below the minimum is rejected")

	_, err = client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("m"), UploadId: created.UploadId})
	require.NoError(t, err)
	assert.Equal(t, 1, srv.AbortedUploads())
	assert.Empty(t, srv.OpenUploads())
	assert.Empty(t, srv.Keys())
}

func TestTaggingMetadataAndHead(t *testing.T) {
	srv := NewServer(t)
	client := srv.Client()
	_, err := putObject(t, client, srv, "k", "body", func(in *s3.PutObjectInput) {
		in.Tagging = aws.String("a=1&b=2")
		in.Metadata = map[string]string{"Owner": "me"}
		in.ContentType = aws.String("text/plain")
	})
	require.NoError(t, err)
	head, err := client.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("k")})
	require.NoError(t, err)
	assert.Equal(t, int64(4), aws.ToInt64(head.ContentLength))
	assert.Equal(t, "text/plain", aws.ToString(head.ContentType))
	assert.Equal(t, "me", head.Metadata["owner"])
	tags, err := client.GetObjectTagging(context.Background(), &s3.GetObjectTaggingInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("k")})
	require.NoError(t, err)
	assert.Len(t, tags.TagSet, 2)

	_, err = putObject(t, client, srv, "toomany", "x", func(in *s3.PutObjectInput) {
		in.Tagging = aws.String("a=1&b=2&c=3&d=4&e=5&f=6&g=7&h=8&i=9&j=10&k=11")
	})
	assert.Equal(t, "InvalidTag", apiCode(t, err))
	_, err = client.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("nope")})
	var notFound *types.NotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestRanges(t *testing.T) {
	srv := NewServer(t)
	srv.Seed("k", []byte("0123456789"), nil)
	get := func(r string) (string, error) {
		out, err := srv.Client().GetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("k"), Range: aws.String(r)})
		if err != nil {
			return "", err
		}
		defer out.Body.Close()
		data, _ := io.ReadAll(out.Body)
		return string(data), nil
	}
	got, err := get("bytes=2-4")
	require.NoError(t, err)
	assert.Equal(t, "234", got)
	got, err = get("bytes=8-")
	require.NoError(t, err)
	assert.Equal(t, "89", got)
	_, err = get("bytes=20-30")
	assert.Equal(t, "InvalidRange", apiCode(t, err))
}

func TestUnknownBucketAndZeroLength(t *testing.T) {
	srv := NewServer(t, WithBucket("mine"))
	_, err := srv.Client().PutObject(context.Background(), &s3.PutObjectInput{Bucket: aws.String("other"), Key: aws.String("k"), Body: strings.NewReader("x")})
	assert.Equal(t, "NoSuchBucket", apiCode(t, err))

	_, err = putObject(t, srv.Client(), srv, "empty", "")
	require.NoError(t, err)
	object, ok := srv.Object("empty")
	require.True(t, ok)
	assert.Zero(t, object.Size)
	assert.NotEmpty(t, object.ETag)
}

func TestFaultInjection(t *testing.T) {
	t.Run("skip then fail forever", func(t *testing.T) {
		srv := NewServer(t)
		srv.Inject(Fault{Op: OpPutObject, Skip: 2, Times: -1, Status: http.StatusInternalServerError})
		client := srv.LegacyClient()
		client = s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: client.Options().Credentials, RetryMaxAttempts: 1}, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(srv.Endpoint())
			o.UsePathStyle = true
			o.Retryer = aws.NopRetryer{}
		})
		for i := range 2 {
			_, err := putObject(t, client, srv, fmt.Sprint("ok", i), "x")
			require.NoError(t, err)
		}
		_, err := putObject(t, client, srv, "boom", "x")
		assert.Equal(t, "InternalError", apiCode(t, err))
		_, ok := srv.Object("boom")
		assert.False(t, ok, "a failed request is not applied")
	})
	t.Run("drop after commit applies the write", func(t *testing.T) {
		srv := NewServer(t)
		srv.Inject(Fault{Op: OpPutObject, Action: DropAfterCommit})
		client := noRetryClient(srv)
		_, err := putObject(t, client, srv, "k", "x")
		require.Error(t, err)
		_, ok := srv.Object("k")
		assert.True(t, ok)
		assert.Equal(t, 0, srv.Requests()[0].Status)
		assert.True(t, srv.Requests()[0].Faulted)
	})
	t.Run("drop before commit does not", func(t *testing.T) {
		srv := NewServer(t)
		srv.Inject(Fault{Op: OpPutObject, Action: DropBeforeCommit})
		_, err := putObject(t, noRetryClient(srv), srv, "k", "x")
		require.Error(t, err)
		_, ok := srv.Object("k")
		assert.False(t, ok)
	})
	t.Run("delay", func(t *testing.T) {
		srv := NewServer(t)
		srv.Inject(Fault{Op: OpPutObject, Action: Delay, Delay: 60 * time.Millisecond})
		start := time.Now()
		_, err := putObject(t, srv.Client(), srv, "k", "x")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)
	})
	t.Run("key prefix scoping and clearing", func(t *testing.T) {
		srv := NewServer(t)
		srv.Inject(Fault{KeyPrefix: "bad/", Times: -1, Status: http.StatusBadRequest, Code: "Nope"})
		_, err := putObject(t, srv.Client(), srv, "good/k", "x")
		require.NoError(t, err)
		_, err = putObject(t, srv.Client(), srv, "bad/k", "x")
		assert.Equal(t, "Nope", apiCode(t, err))
		srv.ClearFaults()
		_, err = putObject(t, srv.Client(), srv, "bad/k", "x")
		require.NoError(t, err)
	})
}

func noRetryClient(srv *Server) *s3.Client {
	return s3.NewFromConfig(aws.Config{Region: "us-east-1"}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(srv.Endpoint())
		o.UsePathStyle = true
		o.Retryer = aws.NopRetryer{}
		o.Credentials = srv.Client().Options().Credentials
	})
}

func TestRequestLogRecordsOrderAndHeaders(t *testing.T) {
	srv := NewServer(t)
	_, err := putObject(t, srv.Client(), srv, "a/b", "hello", func(in *s3.PutObjectInput) { in.IfNoneMatch = aws.String("*") })
	require.NoError(t, err)
	_, err = srv.Client().HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(srv.Bucket()), Key: aws.String("a/b")})
	require.NoError(t, err)

	requests := srv.Requests()
	require.Len(t, requests, 2)
	assert.Equal(t, []int{1, 2}, []int{requests[0].Seq, requests[1].Seq})
	assert.Equal(t, OpPutObject, requests[0].Op)
	assert.Equal(t, "a/b", requests[0].Key)
	assert.Equal(t, "*", requests[0].Header.Get("If-None-Match"))
	assert.Equal(t, http.StatusOK, requests[0].Status)
	assert.Equal(t, OpHeadObject, requests[1].Op)
	assert.Len(t, srv.RequestsFor(OpHeadObject), 1)
	srv.ResetRequests()
	assert.Empty(t, srv.Requests())
}

func TestDecodeAWSChunked(t *testing.T) {
	raw := []byte("5;chunk-signature=abc\r\nhello\r\n6\r\n world\r\n0\r\nx-amz-checksum-crc32:AAAA\r\n\r\n")
	got, err := decodeAWSChunked(raw)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(got))
	_, err = decodeAWSChunked([]byte("zz\r\n"))
	require.Error(t, err)
	_, err = decodeAWSChunked([]byte("5\r\nhel"))
	require.Error(t, err)
}

func TestDiscardBodiesKeepsSizesAndETags(t *testing.T) {
	srv := NewServer(t, WithDiscardBodies())
	_, err := putObject(t, srv.Client(), srv, "k", "payload")
	require.NoError(t, err)
	object, ok := srv.Object("k")
	require.True(t, ok)
	assert.Equal(t, int64(7), object.Size)
	assert.Empty(t, object.Data)
	assert.NotEmpty(t, object.ETag)
}

func TestEmulatorFromEnvSkipsWhenUnset(t *testing.T) {
	t.Setenv(EmulatorEndpointEnv, "")
	t.Setenv(EmulatorBucketEnv, "")
	result := t.Run("inner", func(t *testing.T) {
		EmulatorFromEnv(t)
		t.Error("EmulatorFromEnv must skip when the environment is unset")
	})
	assert.True(t, result, "a skipped subtest still counts as passed")

	t.Setenv(EmulatorEndpointEnv, "http://localhost:4566")
	t.Setenv(EmulatorBucketEnv, "b")
	emulator := EmulatorFromEnv(t)
	assert.Equal(t, Emulator{Endpoint: "http://localhost:4566", Bucket: "b", Region: "us-east-1"}, emulator)
}
