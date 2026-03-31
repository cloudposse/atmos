package pro

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultHTTPClientTimeout is the default timeout for HTTP client requests.
	DefaultHTTPClientTimeout = 10 * time.Second
)

// UploadInstances uploads drift detection data to the API.
// Large payloads are automatically split into chunks to stay within server body size limits.
// Each chunk is retried on transient 401/5xx failures with exponential backoff, refreshing
// the OIDC token on 401 errors before each retry.
func (c *AtmosProAPIClient) UploadInstances(dto *dtos.InstancesUploadRequest) error {
	if dto == nil {
		return errors.Join(
			errUtils.ErrFailedToUploadInstances,
			fmt.Errorf("UploadInstances: %w", errUtils.ErrNilRequestDTO),
		)
	}
	endpoint := fmt.Sprintf("%s/%s/instances", c.BaseURL, c.BaseAPIEndpoint)

	// Sanitize instance maps to ensure JSON compatibility.
	// YAML unmarshaling can produce map[interface{}]interface{} in nested settings/vars/env,
	// which encoding/json cannot marshal. Convert them to map[string]interface{}.
	for i := range dto.Instances {
		dto.Instances[i].Settings = sanitizeMapForJSON(dto.Instances[i].Settings)
		dto.Instances[i].Vars = sanitizeMapForJSON(dto.Instances[i].Vars)
		dto.Instances[i].Env = sanitizeMapForJSON(dto.Instances[i].Env)
		dto.Instances[i].Backend = sanitizeMapForJSON(dto.Instances[i].Backend)
		dto.Instances[i].Source = sanitizeMapForJSON(dto.Instances[i].Source)
		dto.Instances[i].Metadata = sanitizeMapForJSON(dto.Instances[i].Metadata)
	}

	// Estimate metadata overhead (everything except the instances array).
	overheadDTO := dtos.InstancesUploadRequest{
		RepoURL:   dto.RepoURL,
		RepoName:  dto.RepoName,
		RepoOwner: dto.RepoOwner,
		RepoHost:  dto.RepoHost,
		Instances: []schema.Instance{},
	}
	overhead := metadataOverhead(overheadDTO)

	return sendChunked(dto.Instances, c.MaxPayloadBytes, overhead, func(chunk []schema.Instance, batch *BatchInfo) error {
		chunkDTO := &dtos.InstancesUploadRequest{
			RepoURL:   dto.RepoURL,
			RepoName:  dto.RepoName,
			RepoOwner: dto.RepoOwner,
			RepoHost:  dto.RepoHost,
			Instances: chunk,
		}
		if batch != nil {
			chunkDTO.BatchID = batch.BatchID
			chunkDTO.BatchIndex = &batch.BatchIndex
			chunkDTO.BatchTotal = &batch.BatchTotal
		}
		return c.sendInstancesRequest(endpoint, chunkDTO)
	})
}

// sendInstancesRequest sends a single instances upload request.
func (c *AtmosProAPIClient) sendInstancesRequest(endpoint string, dto *dtos.InstancesUploadRequest) error {
	// Guard against nil HTTPClient by ensuring a default client with a sane timeout.
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPClientTimeout}
	}

	data, err := json.Marshal(dto)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	// Log safe metadata instead of full payload to prevent secret leakage.
	hash := sha256.Sum256(data)
	log.Debug("Uploading instances DTO.",
		"repo_owner", dto.RepoOwner,
		"repo_name", dto.RepoName,
		"instances_count", len(dto.Instances),
		"payload_bytes", len(data),
		"payload_hash", hex.EncodeToString(hash[:]),
	)

	log.Debug("Uploading instances.", "endpoint", endpoint)

	// Wrap the HTTP call in retry logic to handle transient 401/5xx failures.
	err = doWithRetry("UploadInstances", func() error {
		req, reqErr := getAuthenticatedRequest(c, "POST", endpoint, bytes.NewReader(data))
		if reqErr != nil {
			return errors.Join(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := client.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return errors.Join(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadInstances")
	}, c, defaultRetryConfig())
	if err != nil {
		return errors.Join(errUtils.ErrFailedToUploadInstances, err)
	}

	log.Debug("Uploaded instances.", "endpoint", endpoint)

	return nil
}
