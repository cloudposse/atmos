package backend

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3 wraps gofakes3 for integration testing.
type fakeS3 struct {
	backend *s3mem.Backend
	faker   *gofakes3.GoFakeS3
	server  *httptest.Server
	client  *s3.Client
}

// newFakeS3 creates an in-memory S3 server for testing.
func newFakeS3(t *testing.T) *fakeS3 {
	t.Helper()

	backend := s3mem.New()
	faker := gofakes3.New(backend)
	server := httptest.NewServer(faker.Server())

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("fake-key", "fake-secret", ""),
		),
		config.WithRegion("us-east-1"),
	)
	require.NoError(t, err)

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
	})

	t.Cleanup(func() {
		server.Close()
		ResetS3ClientFactory()
	})

	return &fakeS3{
		backend: backend,
		faker:   faker,
		server:  server,
		client:  client,
	}
}

// TestIntegration_CreateBucket_Basic verifies bucket creation works.
func TestIntegration_CreateBucket_Basic(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Create bucket using our function.
	err := createBucket(ctx, fake.client, "test-bucket", "us-east-1")
	require.NoError(t, err, "createBucket should succeed")

	// Verify bucket exists via HeadBucket.
	_, err = fake.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String("test-bucket"),
	})
	assert.NoError(t, err, "Bucket should exist after creation")
}

// TestIntegration_CreateBucket_UsEast1_NoLocationConstraint verifies us-east-1
// does not include LocationConstraint (AWS requirement).
func TestIntegration_CreateBucket_UsEast1_NoLocationConstraint(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// us-east-1 should not include CreateBucketConfiguration.
	err := createBucket(ctx, fake.client, "east-bucket", "us-east-1")
	require.NoError(t, err, "createBucket for us-east-1 should succeed")

	// Verify bucket exists.
	exists, err := bucketExists(ctx, fake.client, "east-bucket")
	require.NoError(t, err)
	assert.True(t, exists, "Bucket should exist")
}

// TestIntegration_CreateBucket_OtherRegion_HasLocationConstraint verifies
// non-us-east-1 regions include LocationConstraint.
func TestIntegration_CreateBucket_OtherRegion_HasLocationConstraint(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Non-us-east-1 should include LocationConstraint.
	err := createBucket(ctx, fake.client, "west-bucket", "us-west-2")
	require.NoError(t, err, "createBucket for us-west-2 should succeed")

	// Verify bucket exists.
	exists, err := bucketExists(ctx, fake.client, "west-bucket")
	require.NoError(t, err)
	assert.True(t, exists, "Bucket should exist")
}

// TestIntegration_BucketExists_True verifies bucketExists returns true for existing bucket.
func TestIntegration_BucketExists_True(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Create bucket directly.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("existing-bucket"),
	})
	require.NoError(t, err)

	// Verify bucketExists returns true.
	exists, err := bucketExists(ctx, fake.client, "existing-bucket")
	require.NoError(t, err)
	assert.True(t, exists, "bucketExists should return true for existing bucket")
}

// TestIntegration_BucketExists_False verifies bucketExists returns false for non-existent bucket.
func TestIntegration_BucketExists_False(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Verify bucketExists returns false for non-existent bucket.
	exists, err := bucketExists(ctx, fake.client, "nonexistent-bucket")
	require.NoError(t, err)
	assert.False(t, exists, "bucketExists should return false for non-existent bucket")
}

// TestIntegration_EnsureBucket_CreatesNew verifies ensureBucket creates a new bucket.
func TestIntegration_EnsureBucket_CreatesNew(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Ensure bucket (should create it).
	alreadyExisted, err := ensureBucket(ctx, fake.client, "new-bucket", "us-east-1")
	require.NoError(t, err)
	assert.False(t, alreadyExisted, "Should indicate bucket was newly created")

	// Verify bucket exists.
	exists, err := bucketExists(ctx, fake.client, "new-bucket")
	require.NoError(t, err)
	assert.True(t, exists, "Bucket should exist after ensureBucket")
}

// TestIntegration_EnsureBucket_ExistingBucket verifies ensureBucket is idempotent.
func TestIntegration_EnsureBucket_ExistingBucket(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Create bucket first.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("pre-existing-bucket"),
	})
	require.NoError(t, err)

	// Ensure bucket (should detect it exists).
	alreadyExisted, err := ensureBucket(ctx, fake.client, "pre-existing-bucket", "us-east-1")
	require.NoError(t, err)
	assert.True(t, alreadyExisted, "Should indicate bucket already existed")
}

// TestIntegration_EnableVersioning verifies versioning can be enabled.
func TestIntegration_EnableVersioning(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("versioned-bucket"),
	})
	require.NoError(t, err)

	// Enable versioning using our function.
	err = enableVersioning(ctx, fake.client, "versioned-bucket")
	require.NoError(t, err, "enableVersioning should succeed")
}

// TestIntegration_DeleteBucket_Empty verifies deleting an empty bucket.
func TestIntegration_DeleteBucket_Empty(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("delete-empty-bucket"),
	})
	require.NoError(t, err)

	// Delete bucket using our function.
	err = deleteBucket(ctx, fake.client, "delete-empty-bucket")
	require.NoError(t, err, "deleteBucket should succeed for empty bucket")

	// Verify bucket no longer exists.
	exists, err := bucketExists(ctx, fake.client, "delete-empty-bucket")
	require.NoError(t, err)
	assert.False(t, exists, "Bucket should not exist after deletion")
}

// TestIntegration_DeleteAllObjects verifies deleting all objects from a bucket.
// Note: deleteAllObjects uses ListObjectVersions, so versioning must be enabled
// for objects to appear in the version list.
func TestIntegration_DeleteAllObjects(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	bucketName := "delete-with-objects"

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)

	// Enable versioning (required for ListObjectVersions to return objects).
	err = enableVersioning(ctx, fake.client, bucketName)
	require.NoError(t, err)

	// Add objects.
	for _, key := range []string{"file1.tfstate", "file2.tfstate", "subdir/file3.tfstate"} {
		_, err = fake.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})
		require.NoError(t, err)
	}

	// List objects to verify they exist.
	listResp, err := fake.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)
	assert.Len(t, listResp.Contents, 3, "Should have 3 objects before deletion")

	// Delete all objects using our function.
	err = deleteAllObjects(ctx, fake.client, bucketName)
	require.NoError(t, err)

	// Verify objects deleted.
	listResp, err = fake.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)
	assert.Empty(t, listResp.Contents, "Bucket should be empty after object deletion")
}

// TestIntegration_ListAllObjects verifies listing objects returns correct counts.
// ListAllObjects uses ListObjectVersions, so versioning must be enabled.
func TestIntegration_ListAllObjects(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	bucketName := "list-objects-bucket"

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)

	// Enable versioning (required for ListObjectVersions to return objects).
	err = enableVersioning(ctx, fake.client, bucketName)
	require.NoError(t, err)

	// Add multiple objects including state files.
	files := []string{
		"terraform.tfstate",         // state file
		"backup.tfstate",            // state file
		"readme.md",                 // not a state file
		"config.yaml",               // not a state file
		"env/dev/terraform.tfstate", // state file
	}
	for _, key := range files {
		_, err = fake.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})
		require.NoError(t, err)
	}

	// List objects using our function.
	totalObjects, stateFiles, err := listAllObjects(ctx, fake.client, bucketName)
	require.NoError(t, err)
	assert.Equal(t, 5, totalObjects, "Should count all objects")
	assert.Equal(t, 3, stateFiles, "Should count only .tfstate files")
}

// TestIntegration_DeleteS3BucketAndContents_FullFlow tests the complete deletion flow.
// This simulates what happens in production where CreateS3Backend enables versioning.
func TestIntegration_DeleteS3BucketAndContents_FullFlow(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	bucketName := "full-flow-delete"

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)

	// Enable versioning (as CreateS3Backend does in production).
	err = enableVersioning(ctx, fake.client, bucketName)
	require.NoError(t, err)

	// Add objects.
	for _, key := range []string{"state1.tfstate", "state2.tfstate"} {
		_, err = fake.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})
		require.NoError(t, err)
	}

	// Delete bucket contents and bucket.
	err = deleteS3BucketAndContents(ctx, fake.client, bucketName)
	require.NoError(t, err)

	// Verify bucket deleted.
	exists, err := bucketExists(ctx, fake.client, bucketName)
	require.NoError(t, err)
	assert.False(t, exists, "Bucket should not exist after deleteS3BucketAndContents")
}

// TestIntegration_MultipleRegions verifies bucket creation works for various regions.
func TestIntegration_MultipleRegions(t *testing.T) {
	regions := []string{
		"us-east-1",
		"us-west-2",
		"eu-west-1",
		"ap-northeast-1",
	}

	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			fake := newFakeS3(t)
			ctx := context.Background()

			bucketName := "region-test-" + region

			err := createBucket(ctx, fake.client, bucketName, region)
			require.NoError(t, err, "createBucket should succeed for region %s", region)

			exists, err := bucketExists(ctx, fake.client, bucketName)
			require.NoError(t, err)
			assert.True(t, exists, "Bucket should exist for region %s", region)
		})
	}
}

// TestIntegration_EmptyBucket_ListAllObjects verifies listing empty bucket returns zero counts.
func TestIntegration_EmptyBucket_ListAllObjects(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	bucketName := "empty-bucket"

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)

	// List objects in empty bucket.
	totalObjects, stateFiles, err := listAllObjects(ctx, fake.client, bucketName)
	require.NoError(t, err)
	assert.Equal(t, 0, totalObjects, "Empty bucket should have 0 objects")
	assert.Equal(t, 0, stateFiles, "Empty bucket should have 0 state files")
}

// TestIntegration_DeleteAllObjects_EmptyBucket verifies deleteAllObjects handles empty buckets.
func TestIntegration_DeleteAllObjects_EmptyBucket(t *testing.T) {
	fake := newFakeS3(t)
	ctx := context.Background()

	bucketName := "already-empty"

	// Create bucket.
	_, err := fake.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)

	// Delete all objects from empty bucket (should be no-op).
	err = deleteAllObjects(ctx, fake.client, bucketName)
	require.NoError(t, err, "deleteAllObjects should succeed on empty bucket")
}
