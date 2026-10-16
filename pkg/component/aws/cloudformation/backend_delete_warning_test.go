package cloudformation

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
)

func TestDeleteBackendWithParamsStateWarning(t *testing.T) {
	for _, cloudFormation := range []bool{true, false} {
		t.Run(fmt.Sprintf("cloudformation=%t", cloudFormation), func(t *testing.T) {
			deleted := serveBackendDeletion(t)
			config := BuildSyntheticBackendConfig(&targetS3Config{Bucket: "artifacts", Region: "us-east-1"}, nil, nil)
			if !cloudFormation {
				config = map[string]any{"backend_type": "s3", "backend": map[string]any{"bucket": "artifacts", "region": "us-east-1"}}
			}
			var output bytes.Buffer
			t.Cleanup(atmosio.PushUIWriter(&output))
			err := provisioner.DeleteBackendWithParams(&provisioner.DeleteBackendParams{
				Component: "example", Stack: "dev", Force: true,
				DescribeComponent: func(string, string) (map[string]any, error) { return config, nil },
			})
			require.NoError(t, err)
			assert.True(t, deleted.Load(), "must reach actual S3 bucket deletion")
			assert.Contains(t, output.String(), "permanently remove 1 object(s)")
			assert.Contains(t, output.String(), "This action cannot be undone")
			if cloudFormation {
				assert.NotContains(t, output.String(), "Terraform state file(s)")
			} else {
				assert.Contains(t, output.String(), "including 1 Terraform state file(s)")
			}
		})
	}
}

func serveBackendDeletion(t *testing.T) *atomic.Bool {
	t.Helper()
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/artifacts", r.URL.Path)
		w.Header().Set("Content-Type", "application/xml")
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			assert.True(t, r.URL.Query().Has("versions"))
			_, _ = fmt.Fprint(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>false</IsTruncated><Version><Key>example.tfstate</Key><VersionId>1</VersionId></Version></ListVersionsResult>`)
		case http.MethodPost:
			assert.True(t, r.URL.Query().Has("delete"))
			_, _ = fmt.Fprint(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"/>`)
		case http.MethodDelete:
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	backend.SetS3ClientFactory(func(config aws.Config, _ ...func(*s3.Options)) backend.S3ClientAPI {
		config.Credentials = aws.AnonymousCredentials{}
		return s3.NewFromConfig(config, func(options *s3.Options) {
			options.BaseEndpoint = aws.String(server.URL)
			options.UsePathStyle = true
		})
	})
	t.Cleanup(backend.ResetS3ClientFactory)
	return &deleted
}
